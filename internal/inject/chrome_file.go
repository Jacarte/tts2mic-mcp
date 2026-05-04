package inject

import (
    "context"
    "os"
)

var outputPath = "/tmp/tts2mic.wav"

func SetOutputPath(p string) { outputPath = p }

type chromeFile struct{}

func (c *chromeFile) Inject(ctx context.Context, wav []byte) error {
    return os.WriteFile(outputPath, wav, 0644)
}
