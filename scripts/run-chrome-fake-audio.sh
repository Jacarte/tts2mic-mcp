#!/usr/bin/env bash
set -euo pipefail

WAV=${1:-/tmp/tts2mic.wav}
URL=${2:-https://example.com}

chromium \
  --use-fake-ui-for-media-stream \
  --use-fake-device-for-media-stream \
  --use-file-for-fake-audio-capture="$WAV" \
  "$URL"
