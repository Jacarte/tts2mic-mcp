# tts2mic-mcp

A Go MCP server and CLI that inject prepared or synthesized audio through a virtual
microphone. Linux CI uses an isolated PulseAudio server; native macOS uses
BlackHole. The browser continues to use its real `getUserMedia()` capture path.

This project supplies the synthetic user's audio input. UI assertions, application
STT/LLM evaluation, and semantic checks of spoken replies belong in the consuming
application's test suite. The included browser conformance suite verifies both
audio directions without API keys or a voice provider.

## Build

Go 1.25.5 and a C compiler are required (`malgo` uses cgo for the existing macOS
backend). The unit CI matrix builds and runs race tests on Ubuntu and macOS.

```bash
go test -race ./...
go build -o bin/tts2mic-mcp ./cmd/tts2mic-mcp
```

## Linux CI: no hardware or credentials

```bash
docker build -f ci/Dockerfile -t tts2mic-audio-ci .
docker run --name tts2mic-check --init --shm-size=1g --network=none tts2mic-audio-ci
docker cp tts2mic-check:/work/artifacts ./artifacts
docker rm tts2mic-check
```

Copy artifacts even after a failed run. No host audio socket, sound device,
privileged container, or system-wide default-device change is needed. The image
runs the tests as `pwuser`. Image construction downloads dependencies; the test
container itself has networking disabled except for its own loopback interface.
It visits only the trusted local harness, not arbitrary websites.

The image and `playwright-core` package are both pinned to 1.58.2. Update them
together. Browser tests use full Chromium's headless mode and remove Playwright's
`--mute-audio`; they do **not** replace `getUserMedia` or enable fake-device flags.

The CI audio routes are deliberately separate:

```text
tts2mic -> tts2mic_tx sink -> tts2mic_mic source -> browser microphone
browser output -> tts2mic_rx sink -> tts2mic_rx.monitor -> recorder
```

A remapped source exposes the injection sink monitor as a browser-visible input;
renaming a monitor is not sufficient for Chromium. Never feed browser output into
the input sink except in a deliberate feedback test.

The conformance suite checks:

- Browser capture of 16, 24 and 48 kHz PCM16 fixtures, including stereo input,
  pitch/duration checks and a second marker near the end of each clip.
- Reacquisition of the microphone in fresh browser contexts.
- Recording of browser playback through a separate sink, without input feedback.
- Real MCP initialization/tool calls, backend failure reporting, cancellation and
  EOF cleanup. Fixtures and a deterministic tone stub need no credentials.

Tests save input/captured WAVs, browser traces and PulseAudio diagnostics under
`artifacts/audio/`. GitHub Actions uploads them with seven-day retention. One audio
worker runs per container: parallel jobs need separate audio servers/routes.
These controlled tests disable AEC/NS/AGC; they do not prove physical-device
acoustics, production audio processing, STT accuracy or audible speaker quality.

### Run the same harness on Linux without Docker

Install PulseAudio, `pulseaudio-utils`, Node.js 22+, and the Go build prerequisites.
Then:

```bash
go build -o bin/tts2mic-mcp ./cmd/tts2mic-mcp
npm --prefix e2e ci
(cd e2e && npx playwright-core install --with-deps chromium)
bash ci/with-audio.sh
```

`ci/with-audio.sh` creates a private runtime/socket, starts a supervised audio server,
checks readiness and cleans it up when its child command exits. It can wrap a
consumer application's test command instead:

```bash
bash ci/with-audio.sh your-test-command
```

For tests that call real voice services, permit network access and supply narrowly
scoped credentials in a separate trusted CI job. The browser and injector must
share the audio server; a local injector cannot speak into an unrelated remote
browser.

## Local backends

Backend precedence is explicit `--target` / MCP `target`, then `TTS2MIC_BACKEND`,
then the OS default (`pipewire` on Linux, `macos-blackhole` on macOS). Unknown
backends fail instead of silently falling back. Other native OS defaults are not
implemented; Windows hosts can run the Linux container where supported.

### Linux / PipeWire

The `pipewire` name denotes the PulseAudio-compatible backend: it works with
PulseAudio or `pipewire-pulse`. Native PipeWire operation is not exercised by the
container CI; that lane deliberately uses PulseAudio directly.

```bash
bash scripts/setup-pipewire-virtual-mic.sh
export TTS2MIC_BACKEND=pipewire
export TTS2MIC_PULSE_SINK=tts2mic_tx
# Set these for the browser process, not as a replacement for the injector sink:
# PULSE_SOURCE=tts2mic_mic PULSE_SINK=tts2mic_rx your-browser-command
bin/tts2mic-mcp play --file fixtures/command.wav
```

The setup script creates a remapped microphone and separate reply sink without
changing desktop defaults. Select `TTS2Mic` in the application where supported.
`TTS2MIC_PULSE_SINK` takes precedence over the legacy `PULSE_SINK` routing setting;
without either, the injector explicitly targets `tts2mic_tx`, not system speakers.
The backend parses PCM16 WAV chunks and passes the actual rate/channel count to
`paplay`. Unknown chunks/padding are supported; compressed or malformed WAVs fail.

### macOS / BlackHole

```bash
bash scripts/setup-macos-blackhole.sh
export TTS2MIC_BACKEND=macos-blackhole
export TTS2MIC_MACOS_OUTPUT_DEVICE="BlackHole 2ch"
bin/tts2mic-mcp play --file fixtures/command.wav
```

Select the matching BlackHole microphone in the browser. The existing `malgo`
backend targets the playback device directly; it does not require changing the
system output. Native device delivery remains a local/manual test: the macOS CI
job covers builds/unit tests, not a provisioned BlackHole driver. Keep
`TTS2MIC_MACOS_DEBUG_AFPLAY` and `TTS2MIC_ALLOW_SYSTEM_OUTPUT_ROUTE` unset for timed
tests, since the debug route plays via system output before device injection.

### Chromium file mode

`chrome-file` only writes a WAV; it does not control a live browser stream.

```bash
bin/tts2mic-mcp speak --target chrome-file --text "Hello" --out /tmp/input.wav
bash scripts/run-chrome-fake-audio.sh /tmp/input.wav http://localhost:3000
```

Use that mode for separate fake-capture smoke tests, not the device-backed CI lane.

## CLI: prepare independently of playback

```bash
# Select a real TTS provider for words; the default stub emits a tone.
export TTS_PROVIDER=elevenlabs
export ELEVENLABS_API_KEY=...
export ELEVENLABS_VOICE_ID=...
export ELEVENLABS_OUTPUT_FORMAT=pcm_16000

bin/tts2mic-mcp prepare --text "Show my calendar" --out /tmp/calendar.wav
bin/tts2mic-mcp play --target pipewire --file /tmp/calendar.wav
bin/tts2mic-mcp speak --text "Show my calendar" --delay 500ms
```

`speak` combines synthesis and injection. CLI commands are supervised and return
nonzero on failure. `prepare` writes a WAV without opening an audio device. `play`
accepts a regular PCM16 WAV file of at most 64 MiB, so human-recorded fixtures do not
need a provider. Delay starts **after** synthesis, immediately before injection;
pre-prepare clips when timing matters. A two-minute deadline includes preparation,
delay and playback. SIGINT/SIGTERM cancels work.

CLI stdout contains a JSON result; diagnostics go to stderr. `completed` means the
backend returned successfully, not that the browser captured or the user heard the
clip. In `chrome-file` mode it means the file was written. For BlackHole, the current
completion boundary is sample submission/device stop, not independently measured
playout; use capture-based checks to detect tail loss.

## MCP interface

Run `bin/tts2mic-mcp` (or `bin/tts2mic-mcp serve`) as a stdio MCP server. Use a proper
MCP client: initialize the JSON-RPC connection, send `notifications/initialized`,
then call `tools/call`. The old raw `{name, arguments}` stdin example was not a
complete MCP exchange.

Tools:

| Tool | Arguments | Result |
| --- | --- | --- |
| `prepare` | `text`, optional `voice` | Content-addressed `clip_id`, duration, rate, channels; no playback. |
| `play` | `clip_id`, optional `target`, `delay` | Supervised job snapshot. |
| `speak` | `text`, optional `voice`, `target`, `delay` | Supervised job snapshot. |
| `speak_delay` | `text`, `delay_ms`, optional `voice`, `target` | Compatibility alias; `delay_ms` is still a duration string such as `500ms`. |
| `job_status` | `id` | Current job state and timestamps/error. |
| `cancel_job` | `id` | Cancellation requested; poll for terminal state. |

Example `tools/call` after initialization:

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"speak","arguments":{"text":"Show my calendar","target":"pipewire"}}}
```

Results include structured content and equivalent JSON text. `speak` now returns a
job object rather than the old `"ok"` string. Accepted jobs run serially, with at
most 16 queued and the last 128 terminal jobs retained. States are `queued`,
`running`, `cancelling`, `completed`, `failed`, and `cancelled`. `running` can include
synthesis or a delay; it is **not** an audible-start event. The two-minute deadline
starts at submission, so queue time counts. On cancellation of a queued job, it
becomes terminal when the worker reaches it; it never plays. EOF/server shutdown
cancels and joins all outstanding work instead of leaving detached children.

Prepare clips before timed conversations, then trigger `play` after the browser
observer establishes the desired condition. An application testing agent should
observe actual captured output and UI state, not infer success from a job result.

## Configuration and retention

`TTS_PROVIDER` is `stub` (also the unset default) or `elevenlabs`; other values fail
at the CLI/MCP boundary. The stub emits a 440 Hz tone, not speech. ElevenLabs also
uses `ELEVENLABS_MODEL_ID` (default `eleven_multilingual_v2`),
`ELEVENLABS_VOICE_ID`, `ELEVENLABS_API_KEY`, and `ELEVENLABS_OUTPUT_FORMAT`.
Use raw PCM output formats, not MP3. Provider/cache behavior beyond the portable
audio work is unchanged in this PR; in particular, the existing synthesis cache
key does not include the model ID. Use separate `TTS_CACHE_DIR` values when changing
models until that cache contract is expanded.

`TTS_CACHE_DIR` defaults to `.tts2mic-cache`, with `TTS_PROVIDER_NAME` and `TTS_LANG`
participating in the existing cache key. `TTS2MIC_CLIP_DIR` defaults to
`.tts2mic-clips`; prepared clip IDs hash the exact WAV bytes, and loading checks the
hash. Clip storage has no automatic retention policy: delete test caches/clips as
part of environment cleanup. Recorded speech can be sensitive; use synthetic
fixtures or consented recordings, do not commit credentials, and define artifact
retention in consuming projects.

A `.env` beside the compiled executable is still read; inherited environment
variables win. The Go dependency set remains unchanged. No secrets, transcript
text or audio are written to MCP stdout except explicit tool results.

MIT licensed — see [LICENSE](LICENSE).
