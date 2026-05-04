package inject

import (
    "bytes"
    "context"
    "os/exec"
)

// PipeWire/PulseAudio compatible backend using pactl/paplay
// It plays the WAV to a virtual sink so its monitor becomes a microphone.

type pipewire struct{}

func (p *pipewire) Inject(ctx context.Context, wav []byte) error {
    // paplay reads from stdin and sends to default sink or specified sink via env PULSE_SINK
    cmd := exec.CommandContext(ctx, "paplay", "--raw", "--rate=16000", "--channels=1", "--format=s16le")

    // If user set PULSE_SINK env var, paplay will route to that sink (e.g. tts2mic_sink)
    cmd.Stdin = bytes.NewReader(wavToPCM(wav))

    return cmd.Run()
}

// naive extraction of PCM from WAV (skip header). This is fine for our generated files.
func wavToPCM(wav []byte) []byte {
    if len(wav) <= 44 {
        return wav
    }
    return wav[44:]
}
