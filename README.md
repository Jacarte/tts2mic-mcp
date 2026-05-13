# tts2mic-mcp

`tts2mic-mcp` is a Go-based MCP server for end-to-end testing browser voice applications by injecting TTS-generated audio into a simulated microphone.

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
- MCP server over stdio using `mcp-go`
- `speak` tool
- pluggable TTS provider interface
- deterministic local TTS stub
- ElevenLabs provider
- filesystem PCM cache
- WAV encoding
- macOS BlackHole backend with direct device-targeted playback
- Linux PipeWire/PulseAudio backend
- Chrome fake-audio-file backend
- setup scripts
- GitHub Actions CI

It is still intentionally small, but the core MCP transport is already in place.

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

## How to use as an MCP server

The server runs over stdio using `mcp-go` and currently exposes a single tool, `speak`.

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
The `macos-blackhole` backend now opens a playback device directly through CoreAudio via `malgo`, so it does not need the current macOS system output to be switched just to inject speech.

Install/setup:

```bash
./scripts/setup-macos-blackhole.sh
```

Then:

1. Open **Audio MIDI Setup**.
2. Confirm `BlackHole 2ch` exists.
3. In your browser app, select `BlackHole 2ch` as the microphone.
4. Inject speech (sine test without provider):

```bash
go run ./cmd/tts2mic-mcp speak \
  --target macos-blackhole \
  --text "hello world"
```

By default the backend looks for the first playback device whose name contains `BlackHole`.
You can override that lookup if needed:

```bash
export TTS2MIC_MACOS_OUTPUT_DEVICE="BlackHole 2ch"
```


As a use case, in Playwright, your app can select the mic the same way a user would. For apps that expose an input selector, select `BlackHole 2ch`. For apps using `navigator.mediaDevices.enumerateDevices()`, choose the audio input whose label contains `BlackHole` after microphone permission is granted.

## Linux: simulated microphone with PipeWire/PulseAudio (not tested yet)

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

The project currently supports two providers:

- **Default test provider**: a deterministic local sine-wave stub used when `TTS_PROVIDER` is unset or set to anything other than `elevenlabs`
- **Real provider**: ElevenLabs, selected with `TTS_PROVIDER=elevenlabs`

Default local test setup:

```bash
unset TTS_PROVIDER
export TTS_PROVIDER_NAME=stub
export TTS_LANG=en-US
```

ElevenLabs setup:

```bash
export TTS_PROVIDER=elevenlabs
export ELEVENLABS_API_KEY=...
export ELEVENLABS_VOICE_ID=...
export ELEVENLABS_MODEL_ID=eleven_multilingual_v2
export ELEVENLABS_OUTPUT_FORMAT=pcm_16000
```

Notes:

- `ELEVENLABS_OUTPUT_FORMAT` defaults to `pcm_16000`.
- If no `voice` is passed to `speak`, the ElevenLabs backend falls back to `ELEVENLABS_VOICE_ID`.
- `TTS_PROVIDER_NAME` is used as part of the cache key and defaults to `stub` or the value of `TTS_PROVIDER`.

## Roadmap

- Add additional real TTS providers beyond ElevenLabs, such as Azure or OpenAI.
- Add richer device controls for the macOS CoreAudio backend, such as explicit device listing and selection helpers.
- Add 48 kHz resampling for browser/STT realism.
- Add Playwright-oriented helpers for browser E2E flows, such as selecting the simulated microphone and asserting setup state.
- Add transcript assertion helpers.
- Add higher-level MCP/testing ergonomics on top of the existing server, such as richer tool metadata and reusable test helpers.
- Add audio fixtures for noise, silence, barge-in, long utterances, and multi-turn flows.
