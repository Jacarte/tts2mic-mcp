// Browser labels describe devices; they do not identify the route used by a
// generic "default"/"communications" endpoint. Select a concrete input instead.
export const audioConstraints = Object.freeze({
  echoCancellation: false,
  noiseSuppression: false,
  autoGainControl: false,
});

export function selectTestMicrophone(devices) {
  const candidates = devices.filter(device =>
    device.kind === 'audioinput' &&
    device.deviceId &&
    !['default', 'communications'].includes(device.deviceId) &&
    /TTS2Mic/i.test(device.label));
  if (candidates.length !== 1) {
    throw new Error(`Expected exactly one concrete TTS2Mic microphone; found ${candidates.length}. Devices: ${JSON.stringify(devices)}`);
  }
  return candidates[0];
}

// The short-lived probe grants permission/unlocks labels, then is stopped before
// the real capture is acquired. Never silently fall back to a default device.
// diagnostics is retained by the harness even when acquisition fails.
export async function openTestMicrophone(mediaDevices, diagnostics) {
  let probe;
  try {
    probe = await mediaDevices.getUserMedia({audio: {...audioConstraints}});
    const track = probe.getAudioTracks()[0];
    diagnostics.probe = {label: track.label, settings: track.getSettings()};
    diagnostics.devices = (await mediaDevices.enumerateDevices()).map(device => ({
      kind: device.kind, deviceId: device.deviceId, label: device.label,
      groupId: device.groupId,
    }));
    diagnostics.selected = selectTestMicrophone(diagnostics.devices);
  } finally {
    probe?.getTracks().forEach(track => track.stop());
  }

  const stream = await mediaDevices.getUserMedia({audio: {
    ...audioConstraints,
    deviceId: {exact: diagnostics.selected.deviceId},
  }});
  try {
    const track = stream.getAudioTracks()[0];
    diagnostics.captured = {label: track.label, settings: track.getSettings()};
    if (diagnostics.captured.settings.deviceId !== diagnostics.selected.deviceId) {
      throw new Error(`Captured microphone ID does not match the requested device: ${JSON.stringify(diagnostics)}`);
    }
    return stream;
  } catch (error) {
    stream.getTracks().forEach(track => track.stop());
    throw error;
  }
}
