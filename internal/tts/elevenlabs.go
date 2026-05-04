package tts

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strconv"

    "github.com/Jacarte/tts2mic-mcp/internal/cache"
)

// ElevenLabs provider.
// Requests raw PCM directly from ElevenLabs using output_format.
// Default output format: pcm_16000

const elevenlabsURL = "https://api.elevenlabs.io/v1/text-to-speech/%s?output_format=%s"

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

    outputFormat := os.Getenv("ELEVENLABS_OUTPUT_FORMAT")
    if outputFormat == "" {
        outputFormat = "pcm_16000"
    }
    sampleRate := elevenLabsSampleRate(outputFormat)

    st := cacheStore()
    key := cache.NewKey(lang(), voice, providerName()+"-"+outputFormat, text)

    if b, ok, err := st.Get(key); err == nil && ok {
        return bytesToInt16(b), sampleRate, nil
    } else if err != nil {
        return nil, 0, err
    }

    modelID := os.Getenv("ELEVENLABS_MODEL_ID")
    if modelID == "" {
        modelID = "eleven_multilingual_v2"
    }

    payload := map[string]any{
        "text": text,
        "model_id": modelID,
        "voice_settings": map[string]any{
            "stability": 0.5,
            "similarity_boost": 0.75,
        },
    }

    body, _ := json.Marshal(payload)

    req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf(elevenlabsURL, voice, outputFormat), bytes.NewReader(body))
    if err != nil {
        return nil, 0, err
    }

    req.Header.Set("xi-api-key", apiKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "audio/pcm")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, 0, err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, 0, fmt.Errorf("elevenlabs error: %s", string(b))
    }

    pcmBytes, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, 0, err
    }

    pcm := bytesToInt16(pcmBytes)
    if pcm == nil {
        return nil, 0, fmt.Errorf("elevenlabs returned invalid PCM byte length: %d", len(pcmBytes))
    }

    _ = st.Set(key, pcmBytes)

    return pcm, sampleRate, nil
}

func elevenLabsSampleRate(outputFormat string) int {
    // Expected forms include pcm_16000, pcm_22050, pcm_24000, pcm_44100.
    const prefix = "pcm_"
    if len(outputFormat) <= len(prefix) || outputFormat[:len(prefix)] != prefix {
        return 16000
    }
    sr, err := strconv.Atoi(outputFormat[len(prefix):])
    if err != nil || sr <= 0 {
        return 16000
    }
    return sr
}
