package inject

import (
    "context"
    "fmt"
    "os"
    "os/exec"
)

// macosBlackhole plays generated audio to the current macOS output device.
// It does NOT write to an input device directly.
//
// To make this act as a simulated microphone, the current macOS output device
// must be routed to a virtual loopback device such as BlackHole, and the browser
// must select that same BlackHole device as its microphone.
//
// In other words:
//   afplay -> macOS selected output -> BlackHole -> browser selected input
//
// For direct per-device output without changing system output, this backend
// should be replaced with a CoreAudio/PortAudio writer that explicitly opens
// the BlackHole output device.
type macosBlackhole struct{}

func (m *macosBlackhole) Inject(ctx context.Context, wav []byte) error {
    if os.Getenv("TTS2MIC_ALLOW_SYSTEM_OUTPUT_ROUTE") != "1" {
        return fmt.Errorf("macos-blackhole uses afplay and only writes to the current macOS output device; set system output to BlackHole and export TTS2MIC_ALLOW_SYSTEM_OUTPUT_ROUTE=1, or implement a direct CoreAudio/PortAudio output backend")
    }

    tmp := "/tmp/tts2mic.wav"
    if err := os.WriteFile(tmp, wav, 0644); err != nil {
        return err
    }

    cmd := exec.CommandContext(ctx, "afplay", tmp)
    return cmd.Run()
}
