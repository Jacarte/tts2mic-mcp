# tts2mic-mcp

`tts2mic-mcp` is a small Go scaffold for end-to-end testing browser voice applications.

The idea is:

```text
MCP tool: speak(target, text)
        ↓
TTS provider
        ↓
WAV / PCM audio
        ↓
Browser microphone simulation backend
        ↓
Browser app using getUserMedia / WebRTC / STT
```

This repo starts with two practical injection modes:

1. **Chrome fake audio file** — deterministic and CI-friendly using Chromium flags.
2. **PipeWire/PulseAudio virtual microphone** — useful for manual and real-browser testing on Linux.

## Current status

This is a scaffold, not a finished production package. It includes:

- a Go CLI/server entrypoint
- a simple JSON-over-stdio tool loop compatible with MCP-style tool invocation experiments
- a pluggable TTS provider interface
- a local sine-wave TTS stub for development
- WAV writer utilities
- browser injection backend abstractions
- Linux virtual mic setup script
- Playwright example
- GitHub Actions CI

## Quick start

```bash
go test ./...
go run ./cmd/tts2mic-mcp --help
```

Generate a test WAV through the local stub:

```bash
go run ./cmd/tts2mic-mcp speak --target chrome-file --text "hello world" --out /tmp/tts2mic.wav
```

Run Chrome/Chromium with the generated WAV as fake microphone input:

```bash
./scripts/run-chrome-fake-audio.sh /tmp/tts2mic.wav https://example.com
```

## Tool shape

The main intended tool is:

```json
{
  "name": "speak",
  "arguments": {
    "target": "chrome-file",
    "text": "hello world",
    "voice": "default"
  }
}
```

The Go type is:

```go
type SpeakInput struct {
    Target string `json:"target"`
    Text   string `json:"text"`
    Voice  string `json:"voice,omitempty"`
}
```

## Injection modes

### `chrome-file`

Creates a WAV file that can be passed to Chrome/Chromium:

```bash
chromium \
  --use-fake-ui-for-media-stream \
  --use-fake-device-for-media-stream \
  --use-file-for-fake-audio-capture=/tmp/tts2mic.wav
```

This is the recommended mode for deterministic browser automation.

### `pipewire`

The scaffold includes scripts for setting up a virtual microphone sink/monitor on Linux.

```bash
./scripts/setup-pipewire-virtual-mic.sh
```

Then select the monitor source as the microphone in your browser or test profile.

## Environment variables

Future real TTS providers should be configured through environment variables, for example:

```bash
export TTS_PROVIDER=azure
export AZURE_SPEECH_KEY=...
export AZURE_SPEECH_REGION=...
export AZURE_SPEECH_VOICE=en-US-JennyNeural
```

The current scaffold defaults to a local deterministic tone generator so the project builds and tests without cloud credentials.

## Roadmap

- Add a real MCP SDK transport once the target client/runtime is decided.
- Add Azure / ElevenLabs / OpenAI TTS providers.
- Add real-time PipeWire writer backend.
- Add transcript assertion helpers for Playwright.
- Add fixtures for noisy audio, silence, barge-in, and multi-turn flows.
