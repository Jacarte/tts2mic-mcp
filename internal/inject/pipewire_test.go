package inject

import (
	"github.com/Jacarte/tts2mic-mcp/internal/audio"
	"slices"
	"testing"
)

func TestPulseArgsPreserveFormatAndExplicitSink(t *testing.T) {
	for _, sampleRate := range []int{16000, 24000, 48000} {
		wav, err := audio.EncodeWAV([]int16{1, -2, 3, -4}, sampleRate, 2)
		if err != nil {
			t.Fatal(err)
		}
		args, pcm, err := pulseArgs(wav, "test_tx")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(args, "--device=test_tx") || !slices.Contains(args, "--channels=2") || len(pcm) != 8 {
			t.Fatalf("%v %v", args, pcm)
		}
		// Non-16 kHz input must not be relabeled.
		expected := map[int]string{16000: "--rate=16000", 24000: "--rate=24000", 48000: "--rate=48000"}[sampleRate]
		if !slices.Contains(args, expected) {
			t.Fatalf("rate lost: %v", args)
		}
	}
	if _, _, err := pulseArgs([]byte("bad"), ""); err == nil {
		t.Fatal("malformed WAV accepted")
	}
}

func TestTargetResolution(t *testing.T) {
	for _, tc := range []struct{ explicit, env, goos, want string }{
		{"", "", "linux", "pipewire"}, {"", "", "darwin", "macos-blackhole"},
		{"", "chrome-file", "linux", "chrome-file"}, {"pipewire", "macos-blackhole", "darwin", "pipewire"},
	} {
		got, err := ResolveTarget(tc.explicit, tc.env, tc.goos)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %q %v", tc, got, err)
		}
	}
	if _, err := ResolveTarget("typo", "", "linux"); err == nil {
		t.Fatal("unknown backend accepted")
	}
	if _, err := ResolveTarget("", "", "windows"); err == nil {
		t.Fatal("unsupported OS silently routed")
	}
}
