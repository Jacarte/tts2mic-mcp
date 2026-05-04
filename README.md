# tts2mic-mcp

`tts2mic-mcp` is a Go-based MCP-style tool server for end-to-end testing browser voice applications by injecting TTS-generated audio into a simulated microphone.

The intended flow is:

```text
MCP client / test runner
        ↓
speak(target, text, voice)
        ↓
TTS cache lookup
        ↓
TTS provider only on cache miss
        ↓
PCM / WAV audio
        ↓
Virtual microphone backend
        ↓
Browser app using getUserMedia / WebRTC / STT
```

The main use case is testing browser apps where Playwright or another automation tool clicks around normally and selects a simulated microphone device, for example `BlackHole 2ch` on macOS.

## Status

This is an early scaffold. It currently includes:

- Go CLI entrypoint
- simple JSON-over-stdio MCP-style tool loop
- `speak` tool
- pluggable TTS provider interface
- deterministic local TTS stub
- filesystem PCM cache
- WAV encoding
- macOS BlackHole backend
- Linux PipeWire/PulseAudio backend
- Chrome fake-audio-file backend
- setup scripts
- GitHub Actions CI

A real MCP SDK transport and real cloud TTS providers are still TODO.

## Install / build

```bash
go test ./...
go build -o bin/tts2mic-mcp ./cmd/tts2mic-mcp
```

Run the CLI directly:

```bash
go run ./cmd/tts2mic-mcp speak \
  --target chrome-file \
  --text "hello world" \
  --out /tmp/tts2mic.wav
```

## How to use as an MCP-style server

The current scaffold exposes a minimal JSON-over-stdio tool loop. It is intentionally simple so the transport can later be replaced with a real MCP SDK server.

Start the server:

```bash
go run ./cmd/tts2mic-mcp
```

Then send one JSON request per line on stdin:

```json
{"name":"speak","arguments":{"target":"macos-blackhole","text":"hello world","voice":"default"}}
```

Example using `printf`:

```bash
printf '%s\n' '{"name":"speak","arguments":{"target":"macos-blackhole","text":"hello world","voice":"default"}}' \
  | go run ./cmd/tts2mic-mcp
```

Successful response:

```json
{"ok":true}
```

Error response:

```json
{"error":"unknown backend"}
```

## MCP tool: `speak`

### Input schema

```json
{
  "type": "object",
  "required": ["target", "text"],
  "properties": {
    "target": {
      "type": "string",
      "description": "Injection backend to use: macos-blackhole, pipewire, or chrome-file"
    },
    "text": {
      "type": "string",
      "description": "Text to synthesize and inject into the simulated microphone"
    },
    "voice": {
      "type": "string",
      "description": "Provider-specific voice id. Defaults to default. Used in the cache key."
    }
  }
}
```

### Go type

```go
type SpeakInput struct {
    Target string `json:"target"`
    Text   string `json:"text"`
    Voice  string `json:"voice,omitempty"`
}
```

### Example request

```json
{
  "name": "speak",
  "arguments": {
    "target": "macos-blackhole",
    "text": "hello world",
    "voice": "en-US-JennyNeural"
  }
}
```

## macOS: simulated microphone with BlackHole

For browser E2E testing on macOS, use BlackHole so the browser sees a real selectable microphone device.

Install/setup:

```bash
./scripts/setup-macos-blackhole.sh
```

Then:

1. Open **Audio MIDI Setup**.
2. Confirm `BlackHole 2ch` exists.
3. Set system output to `BlackHole 2ch`, or configure an Aggregate/Multi-Output device if you also want to hear audio.
4. In your browser app, select `BlackHole 2ch` as the microphone.
5. Inject speech:

```bash
go run ./cmd/tts2mic-mcp speak \
  --target macos-blackhole \
  --text "hello world"
```

In Playwright, your app can select the mic the same way a user would. For apps that expose an input selector, select `BlackHole 2ch`. For apps using `navigator.mediaDevices.enumerateDevices()`, choose the audio input whose label contains `BlackHole` after microphone permission is granted.

## Linux: simulated microphone with PipeWire/PulseAudio

Create a virtual sink and monitor source:

```bash
./scripts/setup-pipewire-virtual-mic.sh
```

Route playback into the virtual sink:

```bash
export PULSE_SINK=tts2mic_sink
```

Inject speech:

```bash
go run ./cmd/tts2mic-mcp speak \
  --target pipewire \
  --text "hello world"
```

Then select the monitor source as the microphone in the browser.

## Chrome fake-audio-file mode

This mode does not create a normal selectable microphone. It is useful for deterministic Chromium automation but is intentionally less realistic than BlackHole/PipeWire.

Generate a WAV file:

```bash
go run ./cmd/tts2mic-mcp speak \
  --target chrome-file \
  --text "hello world" \
  --out /tmp/tts2mic.wav
```

Run Chromium with fake media flags:

```bash
./scripts/run-chrome-fake-audio.sh /tmp/tts2mic.wav https://example.com
```

## TTS cache

Before calling the TTS provider, the TTS layer checks a filesystem cache.

Logical cache key:

```text
lang:voice_id:provider:sha256(text)
```

Default cache directory:

```text
.tts2mic-cache/
```

The cache stores raw little-endian `int16` PCM files and is gitignored.

Example layout:

```text
.tts2mic-cache/
  stub/
    und/
      default/
        und__default__stub__<sha256>.pcm
```

Environment variables:

```bash
export TTS_CACHE_DIR=.tts2mic-cache
export TTS_PROVIDER_NAME=stub
export TTS_LANG=en-US
```

Current behavior:

```text
first speak call  → cache miss → provider called → PCM stored
next same call    → cache hit  → provider skipped
```

When real providers are added, the cache key should also include any options that affect output audio, such as sample rate, speaking rate, pitch, style, and output format.

## TTS providers

The current provider is a deterministic local sine-wave stub so the project builds and tests without credentials.

Future providers can be configured with environment variables, for example:

```bash
export TTS_PROVIDER=azure
export AZURE_SPEECH_KEY=...
export AZURE_SPEECH_REGION=...
export AZURE_SPEECH_VOICE=en-US-JennyNeural
```

## Roadmap

- Replace the JSON-over-stdio loop with a real MCP SDK server.
- Add Azure / ElevenLabs / OpenAI TTS providers.
- Add real-time CoreAudio output on macOS instead of `afplay`.
- Add 48 kHz resampling for browser/STT realism.
- Add Playwright helpers for selecting the simulated microphone.
- Add transcript assertion helpers.
- Add audio fixtures for noise, silence, barge-in, long utterances, and multi-turn flows.
