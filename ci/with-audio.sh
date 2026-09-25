#!/usr/bin/env bash
# Own a private PulseAudio server for this command only. Never change a desktop's defaults.
set -euo pipefail
ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
ARTIFACT_DIR=${ARTIFACT_DIR:-"$ROOT/artifacts/audio"}
mkdir -p "$ARTIFACT_DIR"
ARTIFACT_DIR=$(cd "$ARTIFACT_DIR" && pwd)
export ARTIFACT_DIR
runtime=$(mktemp -d)
export XDG_RUNTIME_DIR="$runtime"
export PULSE_RUNTIME_PATH="$runtime/pulse"
mkdir -m 700 "$PULSE_RUNTIME_PATH"
export PULSE_SERVER="unix:$PULSE_RUNTIME_PATH/native"
export PULSE_SOURCE=tts2mic_mic
export PULSE_SINK=tts2mic_rx
export TTS2MIC_BACKEND=pipewire
export TTS2MIC_PULSE_SINK=tts2mic_tx
pulse_pid= child_pid=
cleanup() {
  trap - EXIT
  [[ -z "$child_pid" ]] || kill "$child_pid" 2>/dev/null || true
  [[ -z "$pulse_pid" ]] || kill "$pulse_pid" 2>/dev/null || true
  [[ -z "$child_pid" ]] || wait "$child_pid" 2>/dev/null || true
  [[ -z "$pulse_pid" ]] || wait "$pulse_pid" 2>/dev/null || true
  rm -rf -- "$runtime"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
pulseaudio --daemonize=no --use-pid-file=no --exit-idle-time=-1 \
  --log-target="file:$ARTIFACT_DIR/pulseaudio.log" -n --file="$ROOT/ci/pulse.pa" &
pulse_pid=$!
ready=false
for _ in {1..100}; do
  if pactl info >"$ARTIFACT_DIR/pulse-info.txt" 2>/dev/null; then ready=true; break; fi
  if ! kill -0 "$pulse_pid" 2>/dev/null; then break; fi
  sleep 0.1
done
if [[ "$ready" != true ]]; then
  cat "$ARTIFACT_DIR/pulseaudio.log" >&2 || true
  echo 'Private PulseAudio server did not become ready' >&2
  exit 1
fi
pactl list short sinks >"$ARTIFACT_DIR/sinks.txt"
pactl list short sources >"$ARTIFACT_DIR/sources.txt"
grep -q $'\ttts2mic_mic\t' "$ARTIFACT_DIR/sources.txt"
if [[ $# == 0 ]]; then set -- npm --prefix "$ROOT/e2e" test; fi
"$@" &
child_pid=$!
status=0
wait "$child_pid" || status=$?
child_pid=
exit "$status"
