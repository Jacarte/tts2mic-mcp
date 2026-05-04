package tts

import (
    "context"
    "math"
    "os"
)

// Provider returns mono int16 PCM and sample rate

type Provider interface {
    Synthesize(ctx context.Context, text, voice string) ([]int16, int, error)
}

func NewProviderFromEnv() Provider {
    switch os.Getenv("TTS_PROVIDER") {
    default:
        return &sineProvider{}
    }
}

// Deterministic stub for tests: sine wave "speech"

type sineProvider struct{}

func (s *sineProvider) Synthesize(ctx context.Context, text, voice string) ([]int16, int, error) {
    sr := 16000
    durSec := 1 + len(text)/20
    n := sr * durSec
    out := make([]int16, n)
    freq := 440.0
    for i := 0; i < n; i++ {
        v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sr))
        out[i] = int16(v * 3000)
    }
    return out, sr, nil
}
