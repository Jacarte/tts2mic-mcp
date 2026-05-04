package inject

import (
	"strings"
	"testing"

	"github.com/Jacarte/tts2mic-mcp/internal/audio"
)

func TestDecodePCM16WAV(t *testing.T) {
	pcm := []int16{1, -2, 3, -4}
	wav, err := audio.EncodeWAV(pcm, 16000, 1)
	if err != nil {
		t.Fatalf("EncodeWAV() error = %v", err)
	}

	decoded, err := decodePCM16WAV(wav)
	if err != nil {
		t.Fatalf("decodePCM16WAV() error = %v", err)
	}

	if decoded.sampleRate != 16000 {
		t.Fatalf("sampleRate = %d, want 16000", decoded.sampleRate)
	}
	if decoded.channels != 1 {
		t.Fatalf("channels = %d, want 1", decoded.channels)
	}
	if len(decoded.pcm) != len(pcm)*2 {
		t.Fatalf("pcm bytes = %d, want %d", len(decoded.pcm), len(pcm)*2)
	}
	if decoded.pcm[0] != 1 || decoded.pcm[1] != 0 {
		t.Fatalf("pcm bytes start = %v, want little-endian first sample", decoded.pcm[:2])
	}
}

func TestDecodePCM16WAVRejectsInvalidHeader(t *testing.T) {
	_, err := decodePCM16WAV([]byte("not-a-wav"))
	if err == nil {
		t.Fatal("decodePCM16WAV() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "unsupported wav header") && !strings.Contains(err.Error(), "wav too short") {
		t.Fatalf("decodePCM16WAV() error = %q, want header-related error", err)
	}
}

func TestDecodePCM16WAVRejectsMisalignedPCMData(t *testing.T) {
	wav := []byte{
		'R', 'I', 'F', 'F',
		0x28, 0x00, 0x00, 0x00,
		'W', 'A', 'V', 'E',
		'f', 'm', 't', ' ',
		0x10, 0x00, 0x00, 0x00,
		0x01, 0x00,
		0x02, 0x00,
		0x80, 0x3e, 0x00, 0x00,
		0x00, 0xfa, 0x00, 0x00,
		0x04, 0x00,
		0x10, 0x00,
		'd', 'a', 't', 'a',
		0x03, 0x00, 0x00, 0x00,
		0x01, 0x02, 0x03,
	}

	_, err := decodePCM16WAV(wav)
	if err == nil {
		t.Fatal("decodePCM16WAV() error = nil, want alignment error")
	}
	if !strings.Contains(err.Error(), "not aligned") {
		t.Fatalf("decodePCM16WAV() error = %q, want alignment error", err)
	}
}

func TestFindDeviceNameIndex(t *testing.T) {
	names := []string{"MacBook Pro Speakers", "BlackHole 2ch", "External Monitor"}

	tests := []struct {
		name   string
		wanted string
		index  int
		found  bool
	}{
		{name: "exact case insensitive", wanted: "blackhole 2CH", index: 1, found: true},
		{name: "substring match", wanted: "blackhole", index: 1, found: true},
		{name: "trimmed", wanted: "  external monitor  ", index: 2, found: true},
		{name: "missing", wanted: "Loopback", index: -1, found: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotIndex, gotFound := findDeviceNameIndex(names, tc.wanted)
			if gotFound != tc.found {
				t.Fatalf("findDeviceNameIndex() found = %v, want %v", gotFound, tc.found)
			}
			if gotIndex != tc.index {
				t.Fatalf("findDeviceNameIndex() index = %d, want %d", gotIndex, tc.index)
			}
		})
	}
}

func TestShouldUseAFPlayDebug(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		wanted bool
	}{
		{name: "no flags", env: map[string]string{}, wanted: false},
		{name: "debug flag", env: map[string]string{macOSDebugAFPlayEnv: "1"}, wanted: true},
		{name: "legacy flag", env: map[string]string{"TTS2MIC_ALLOW_SYSTEM_OUTPUT_ROUTE": "1"}, wanted: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tc.env[key]
			}

			got := shouldUseAFPlayDebug(getenv)
			if got != tc.wanted {
				t.Fatalf("shouldUseAFPlayDebug() = %v, want %v", got, tc.wanted)
			}
		})
	}
}
