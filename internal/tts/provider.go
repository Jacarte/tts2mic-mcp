package tts

import (
    "context"
    "encoding/binary"
    "math"
    "os"

    "github.com/Jacarte/tts2mic-mcp/internal/cache"
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

// Cache helpers

func cacheStore() cache.Store {
    return cache.NewStore(os.Getenv("TTS_CACHE_DIR"))
}

func providerName() string {
    if v := os.Getenv("TTS_PROVIDER_NAME"); v != "" {
        return v
    }
    if v := os.Getenv("TTS_PROVIDER"); v != "" {
        return v
    }
    return "stub"
}

func lang() string {
    if v := os.Getenv("TTS_LANG"); v != "" {
        return v
    }
    return "und"
}

// Deterministic stub for tests: sine wave "speech"

type sineProvider struct{}

func (s *sineProvider) Synthesize(ctx context.Context, text, voice string) ([]int16, int, error) {
    st := cacheStore()
    key := cache.NewKey(lang(), voice, providerName(), text)

    if b, ok, err := st.Get(key); err == nil && ok {
        return bytesToInt16(b), 16000, nil
    } else if err != nil {
        return nil, 0, err
    }

    sr := 16000
    durSec := 1 + len(text)/20
    n := sr * durSec
    out := make([]int16, n)
    freq := 440.0
    for i := 0; i < n; i++ {
        v := math.Sin(2 * math.Pi * freq * float64(i) / float64(sr))
        out[i] = int16(v * 3000)
    }

    _ = st.Set(key, int16ToBytes(out))

    return out, sr, nil
}

func int16ToBytes(in []int16) []byte {
    b := make([]byte, len(in)*2)
    for i, v := range in {
        binary.LittleEndian.PutUint16(b[i*2:], uint16(v))
    }
    return b
}

func bytesToInt16(b []byte) []int16 {
    if len(b)%2 != 0 {
        return nil
    }
    out := make([]int16, len(b)/2)
    for i := 0; i < len(out); i++ {
        out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
    }
    return out
}
