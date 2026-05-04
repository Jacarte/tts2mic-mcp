package tts

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"

    "github.com/Jacarte/tts2mic-mcp/internal/cache"
)

// ElevenLabs provider
// Docs: https://api.elevenlabs.io

const elevenlabsURL = "https://api.elevenlabs.io/v1/text-to-speech/%s"

type elevenLabsProvider struct{}

func (e *elevenLabsProvider) Synthesize(ctx context.Context, text, voice string) ([]int16, int, error) {
    apiKey := os.Getenv("ELEVENLABS_API_KEY")
    if apiKey == "" {
        return nil, 0, fmt.Errorf("ELEVENLABS_API_KEY not set")
    }

    if voice == "" {
        voice = os.Getenv("ELEVENLABS_VOICE_ID")
    }
    if voice == "" {
        return nil, 0, fmt.Errorf("voice not provided and ELEVENLABS_VOICE_ID not set")
    }

    // cache
    st := cacheStore()
    key := cache.NewKey(lang(), voice, providerName(), text)

    if b, ok, err := st.Get(key); err == nil && ok {
        return bytesToInt16(b), 16000, nil
    } else if err != nil {
        return nil, 0, err
    }

    payload := map[string]any{
        "text": text,
        "model_id": "eleven_multilingual_v2",
        "voice_settings": map[string]any{
            "stability": 0.5,
            "similarity_boost": 0.75,
        },
    }

    body, _ := json.Marshal(payload)

    req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf(elevenlabsURL, voice), bytes.NewReader(body))
    if err != nil {
        return nil, 0, err
    }

    req.Header.Set("xi-api-key", apiKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "audio/mpeg")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, 0, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, 0, fmt.Errorf("elevenlabs error: %s", string(b))
    }

    mp3Data, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, 0, err
    }

    // NOTE: ElevenLabs returns MP3. For now we return error unless decoded.
    return nil, 0, fmt.Errorf("mp3 decoding not implemented yet (%d bytes)", len(mp3Data))
}
