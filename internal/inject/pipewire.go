package inject

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/Jacarte/tts2mic-mcp/internal/audio"
)

// pipewire uses the PulseAudio protocol, including pipewire-pulse. An explicit
// sink prevents accidental playback through the developer's system speakers.
type pipewire struct{}

func pulseArgs(wav []byte, sink string) ([]string, []byte, error) {
	decoded, err := audio.DecodePCM16(wav)
	if err != nil {
		return nil, nil, err
	}
	if sink == "" {
		sink = "tts2mic_tx"
	}
	args := []string{
		"--device=" + sink, "--raw", "--format=s16le",
		"--rate=" + strconv.Itoa(decoded.SampleRate),
		"--channels=" + strconv.Itoa(decoded.Channels),
	}
	return args, decoded.Samples, nil
}

func (p *pipewire) Inject(ctx context.Context, wav []byte) error {
	sink := os.Getenv("TTS2MIC_PULSE_SINK")
	if sink == "" {
		sink = os.Getenv("PULSE_SINK") // Legacy CLI configuration.
	}
	args, pcm, err := pulseArgs(wav, sink)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "paplay", args...)
	cmd.Stdin = bytes.NewReader(pcm)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("paplay failed (is the virtual sink configured?): %w: %s", err, stderr.String())
	}
	return nil
}
