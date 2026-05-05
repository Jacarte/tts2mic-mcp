package inject

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"
)

const (
	macOSDebugAFPlayEnv    = "TTS2MIC_MACOS_DEBUG_AFPLAY"
	macOSOutputDeviceEnv   = "TTS2MIC_MACOS_OUTPUT_DEVICE"
	defaultBlackHoleDevice = "BlackHole"
	wavHeaderPCMFormat     = 1
	wavHeaderBitsPerSample = 16
)

type macosBlackhole struct{}

func (m *macosBlackhole) Inject(ctx context.Context, wav []byte) error {
	deviceName := os.Getenv(macOSOutputDeviceEnv)
	if deviceName == "" {
		deviceName = defaultBlackHoleDevice
	}

	if shouldUseAFPlayDebug(os.Getenv) {
		playWithAFPlay(ctx, wav)
	}

	return playWAVOnDevice(ctx, wav, deviceName)
}

func playWithAFPlay(ctx context.Context, wav []byte) error {
	tmp := "/tmp/tts2mic.wav"
	if err := os.WriteFile(tmp, wav, 0o644); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "afplay", tmp)
	return cmd.Run()
}

func playWAVOnDevice(ctx context.Context, wav []byte, deviceName string) error {
	decoded, err := decodePCM16WAV(wav)
	if err != nil {
		return err
	}

	audioCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("initialize audio context: %w", err)
	}
	defer func() {
		_ = audioCtx.Uninit()
		audioCtx.Free()
	}()

	playbackDevices, err := audioCtx.Devices(malgo.Playback)
	if err != nil {
		return fmt.Errorf("enumerate playback devices: %w", err)
	}

	playbackDevice, err := findPlaybackDevice(playbackDevices, deviceName)
	if err != nil {
		return err
	}

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.SampleRate = uint32(decoded.sampleRate)
	deviceConfig.Playback.DeviceID = playbackDevice.ID.Pointer()
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = uint32(decoded.channels)

	frameSize := decoded.channels * malgo.SampleSizeInBytes(malgo.FormatS16)
	if frameSize <= 0 {
		return fmt.Errorf("invalid playback frame size for %d channels", decoded.channels)
	}

	pcm := decoded.pcm
	offset := 0
	done := make(chan struct{})
	var doneOnce sync.Once

	device, err := malgo.InitDevice(audioCtx.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: func(output, _ []byte, framecount uint32) {
			requested := int(framecount) * frameSize
			requested = min(requested, len(output))

			remaining := len(pcm) - offset
			copied := 0
			if remaining > 0 && requested > 0 {
				copied = min(requested, remaining)
				copy(output[:copied], pcm[offset:offset+copied])
				offset += copied
			}

			if copied < requested {
				clear(output[copied:requested])
			}

			if offset >= len(pcm) {
				doneOnce.Do(func() { close(done) })
			}
		},
		Stop: func() {
			doneOnce.Do(func() { close(done) })
		},
	})
	if err != nil {
		return fmt.Errorf("initialize playback device %q: %w", playbackDevice.Name(), err)
	}
	defer device.Uninit()

	if err := device.Start(); err != nil {
		return fmt.Errorf("start playback device %q: %w", playbackDevice.Name(), err)
	}

	select {
	case <-ctx.Done():
		_ = device.Stop()
		return ctx.Err()
	case <-done:
		if err := device.Stop(); err != nil {
			return fmt.Errorf("stop playback device %q: %w", playbackDevice.Name(), err)
		}
		return nil
	}
}

type decodedWAV struct {
	pcm        []byte
	sampleRate int
	channels   int
}

func decodePCM16WAV(wav []byte) (decodedWAV, error) {
	if len(wav) < 12 {
		return decodedWAV{}, fmt.Errorf("wav too short")
	}
	if string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return decodedWAV{}, fmt.Errorf("unsupported wav header")
	}

	var (
		formatFound bool
		dataFound   bool
		sampleRate  uint32
		channels    uint16
		pcm         []byte
	)

	for offset := 12; offset+8 <= len(wav); {
		chunkID := string(wav[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(wav[offset+4 : offset+8]))
		chunkStart := offset + 8
		chunkEnd := chunkStart + chunkSize
		if chunkEnd > len(wav) {
			return decodedWAV{}, fmt.Errorf("wav chunk %q exceeds file size", chunkID)
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return decodedWAV{}, fmt.Errorf("wav fmt chunk too short")
			}
			format := binary.LittleEndian.Uint16(wav[chunkStart : chunkStart+2])
			if format != wavHeaderPCMFormat {
				return decodedWAV{}, fmt.Errorf("unsupported wav format %d", format)
			}
			channels = binary.LittleEndian.Uint16(wav[chunkStart+2 : chunkStart+4])
			sampleRate = binary.LittleEndian.Uint32(wav[chunkStart+4 : chunkStart+8])
			bitsPerSample := binary.LittleEndian.Uint16(wav[chunkStart+14 : chunkStart+16])
			if bitsPerSample != wavHeaderBitsPerSample {
				return decodedWAV{}, fmt.Errorf("unsupported wav bits per sample %d", bitsPerSample)
			}
			formatFound = true
		case "data":
			pcm = bytes.Clone(wav[chunkStart:chunkEnd])
			dataFound = true
		}

		offset = chunkEnd
		if chunkSize%2 == 1 {
			offset++
		}
	}

	if !formatFound {
		return decodedWAV{}, fmt.Errorf("wav fmt chunk missing")
	}
	if !dataFound {
		return decodedWAV{}, fmt.Errorf("wav data chunk missing")
	}
	if sampleRate == 0 {
		return decodedWAV{}, fmt.Errorf("wav sample rate missing")
	}
	if channels == 0 {
		return decodedWAV{}, fmt.Errorf("wav channels missing")
	}
	if len(pcm)%(int(channels)*2) != 0 {
		return decodedWAV{}, fmt.Errorf("wav data length %d is not aligned to %d-channel int16 frames", len(pcm), channels)
	}

	return decodedWAV{
		pcm:        pcm,
		sampleRate: int(sampleRate),
		channels:   int(channels),
	}, nil
}

func findPlaybackDevice(devices []malgo.DeviceInfo, wanted string) (*malgo.DeviceInfo, error) {
	names := deviceNames(devices)
	idx, ok := findDeviceNameIndex(names, wanted)
	if !ok {
		return nil, fmt.Errorf("playback device matching %q not found; available playback devices: %s", wanted, strings.Join(names, ", "))
	}

	return &devices[idx], nil
}

func deviceNames(devices []malgo.DeviceInfo) []string {
	names := make([]string, 0, len(devices))
	for _, d := range devices {
		names = append(names, d.Name())
	}
	return names
}

func findDeviceNameIndex(names []string, wanted string) (int, bool) {
	normalizedWanted := normalizeDeviceName(wanted)
	if normalizedWanted == "" {
		return -1, false
	}

	for i, name := range names {
		if normalizeDeviceName(name) == normalizedWanted {
			return i, true
		}
	}

	for i, name := range names {
		if strings.Contains(normalizeDeviceName(name), normalizedWanted) {
			return i, true
		}
	}

	return -1, false
}

func normalizeDeviceName(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func shouldUseAFPlayDebug(getenv func(string) string) bool {
	return getenv(macOSDebugAFPlayEnv) == "1" || getenv("TTS2MIC_ALLOW_SYSTEM_OUTPUT_ROUTE") == "1"
}
