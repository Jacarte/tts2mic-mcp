#!/usr/bin/env bash
set -euo pipefail

# Creates a virtual microphone source that browsers can select via getUserMedia.
# Works on PipeWire systems that provide pactl compatibility.

SINK_NAME=${SINK_NAME:-tts2mic_sink}
SINK_DESCRIPTION=${SINK_DESCRIPTION:-TTS2Mic Speaker}
SOURCE_DESCRIPTION=${SOURCE_DESCRIPTION:-TTS2Mic Simulated Microphone}

if ! command -v pactl >/dev/null 2>&1; then
  echo "pactl is required. Install pulseaudio-utils or pipewire-pulse tooling." >&2
  exit 1
fi

if pactl list short sinks | awk '{print $2}' | grep -qx "$SINK_NAME"; then
  echo "Virtual sink already exists: $SINK_NAME"
else
  pactl load-module module-null-sink \
    sink_name="$SINK_NAME" \
    sink_properties="device.description='$SINK_DESCRIPTION'" >/dev/null
  echo "Created virtual sink: $SINK_NAME"
fi

MONITOR_SOURCE="$SINK_NAME.monitor"

if pactl list short sources | awk '{print $2}' | grep -qx "$MONITOR_SOURCE"; then
  pactl update-source-proplist "$MONITOR_SOURCE" "device.description=$SOURCE_DESCRIPTION" || true
  echo "Simulated microphone source: $MONITOR_SOURCE"
else
  echo "Expected monitor source not found: $MONITOR_SOURCE" >&2
  pactl list short sources >&2
  exit 1
fi

cat <<EOF

Use this microphone in the browser:
  Name: $SOURCE_DESCRIPTION
  Source: $MONITOR_SOURCE

To route generated audio into it, play audio to sink:
  $SINK_NAME

EOF
