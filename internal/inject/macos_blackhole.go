package inject

import (
    "context"
    "os"
    "os/exec"
)

// macOS backend that plays WAV through system output (expected to be BlackHole device)

type macosBlackhole struct{}

func (m *macosBlackhole) Inject(ctx context.Context, wav []byte) error {
    tmp := "/tmp/tts2mic.wav"
    if err := os.WriteFile(tmp, wav, 0644); err != nil {
        return err
    }

    cmd := exec.CommandContext(ctx, "afplay", tmp)
    return cmd.Run()
}
