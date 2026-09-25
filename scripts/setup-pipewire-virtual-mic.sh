#!/usr/bin/env bash
# Desktop setup only. CI uses ci/with-audio.sh and a separate audio server.
set -euo pipefail
SINK_NAME=${SINK_NAME:-tts2mic_tx}
SOURCE_NAME=${SOURCE_NAME:-tts2mic_mic}
OUTPUT_SINK_NAME=${OUTPUT_SINK_NAME:-tts2mic_rx}
command -v pactl >/dev/null || { echo 'Install pulseaudio-utils and start PulseAudio or pipewire-pulse.' >&2; exit 1; }
pactl info >/dev/null
has_node() { pactl list short "$1" | awk '{print $2}' | grep -Fxq -- "$2"; }
if ! has_node sinks "$SINK_NAME"; then
  pactl load-module module-null-sink sink_name="$SINK_NAME" rate=48000 channels=1 channel_map=mono >/dev/null
fi
# Chromium filters monitor devices. Expose the monitor through a normal source.
if ! has_node sources "$SOURCE_NAME"; then
  pactl load-module module-remap-source source_name="$SOURCE_NAME" master="$SINK_NAME.monitor" \
    channels=1 channel_map=mono master_channel_map=mono source_properties=device.description=TTS2Mic >/dev/null
fi
if ! has_node sinks "$OUTPUT_SINK_NAME"; then
  pactl load-module module-null-sink sink_name="$OUTPUT_SINK_NAME" rate=48000 channels=2 >/dev/null
fi
cat <<MSG
Virtual microphone: $SOURCE_NAME (TTS2Mic)
Injection sink:     $SINK_NAME
Reply output sink:  $OUTPUT_SINK_NAME

Injector: TTS2MIC_BACKEND=pipewire TTS2MIC_PULSE_SINK=$SINK_NAME
Browser:  PULSE_SOURCE=$SOURCE_NAME PULSE_SINK=$OUTPUT_SINK_NAME
Recorder: parec --device=$OUTPUT_SINK_NAME.monitor --raw --format=s16le --rate=48000 --channels=1

Existing system defaults were not changed. Select TTS2Mic in the app where supported.
For isolated automated tests, use ci/with-audio.sh instead.
MSG
