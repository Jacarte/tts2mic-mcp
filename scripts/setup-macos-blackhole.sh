#!/usr/bin/env bash
set -euo pipefail

# macOS virtual microphone setup using BlackHole.
# This script installs BlackHole via Homebrew if needed and prints the manual routing steps.
# macOS audio routing often requires user approval and Audio MIDI Setup configuration.

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "This script is for macOS only." >&2
  exit 1
fi

if ! command -v brew >/dev/null 2>&1; then
  echo "Homebrew is required to install BlackHole automatically." >&2
  echo "Install Homebrew or install BlackHole manually from: https://github.com/ExistentialAudio/BlackHole" >&2
  exit 1
fi

if ! system_profiler SPAudioDataType 2>/dev/null | grep -q "BlackHole"; then
  echo "Installing BlackHole 2ch..."
  brew install blackhole-2ch
else
  echo "BlackHole appears to already be installed."
fi

cat <<'EOF'

Next steps in macOS:

1. Open "Audio MIDI Setup".
2. Confirm that "BlackHole 2ch" exists.
3. In your browser app, select "BlackHole 2ch" as the microphone.
4. From tts2mic-mcp, inject audio with:

   go run ./cmd/tts2mic-mcp speak --target macos-blackhole --text "hello world"

The backend uses `afplay` to play generated WAV audio into the selected output device.
For deterministic routing, set your system output device to "BlackHole 2ch" before running the test,
or create a Multi-Output/Aggregate Device in Audio MIDI Setup if you also want to hear the audio.

EOF
