import { test } from 'node:test';
import assert from 'node:assert/strict';
import { audioConstraints, openTestMicrophone, selectTestMicrophone } from './microphone.mjs';

const device = (deviceId, label = 'TTS2Mic', kind = 'audioinput') => ({
  deviceId, label, kind, groupId: 'test-group',
});
const devices = [
  device('default', 'Default'),
  device('communications', 'Communications - TTS2Mic'),
  device('output', 'TTS2Mic', 'audiooutput'),
  device('mic-id'),
];

function fakeStream(label, deviceId) {
  const track = {
    label, stopped: false,
    getSettings: () => ({deviceId}),
    stop() { this.stopped = true; },
  };
  return {track, getAudioTracks: () => [track], getTracks: () => [track]};
}

function fixture({listed = devices, capturedID = 'mic-id', failure} = {}) {
  const probe = fakeStream('Default', 'default');
  const captured = fakeStream('TTS2Mic', capturedID);
  const calls = [];
  const mediaDevices = {
    async getUserMedia(constraints) {
      calls.push(constraints);
      if (calls.length === 1) return probe;
      assert.equal(probe.track.stopped, true, 'probe must stop before reacquiring');
      if (failure) throw failure;
      return captured;
    },
    async enumerateDevices() {
      assert.equal(probe.track.stopped, false, 'enumerate while labels are available');
      return listed;
    },
  };
  return {mediaDevices, probe, captured, calls};
}

test('selects a concrete input instead of default/communications aliases or output devices', () => {
  assert.equal(selectTestMicrophone(devices).deviceId, 'mic-id');
  assert.equal(selectTestMicrophone([device('default', 'Default - TTS2Mic'), device('real')]).deviceId, 'real');
});

test('rejects missing, unlabelled and ambiguous microphones without default fallback', () => {
  for (const listed of [
    [], [device('default', 'Default - TTS2Mic')], [device('real', '')],
    [device('one'), device('two')], [device('', 'TTS2Mic')],
  ]) assert.throws(() => selectTestMicrophone(listed), /Expected exactly one concrete TTS2Mic microphone/);
});

test('Default probe label is accepted, but capture uses and verifies the exact concrete ID', async () => {
  const f = fixture();
  const diagnostics = {};
  const stream = await openTestMicrophone(f.mediaDevices, diagnostics);
  assert.equal(stream, f.captured);
  assert.deepEqual(f.calls, [
    {audio: {...audioConstraints}},
    {audio: {...audioConstraints, deviceId: {exact: 'mic-id'}}},
  ]);
  assert.equal(diagnostics.probe.label, 'Default');
  assert.equal(diagnostics.selected.deviceId, 'mic-id');
  assert.equal(diagnostics.captured.settings.deviceId, 'mic-id');
  assert.equal(f.probe.track.stopped, true);
  assert.equal(f.captured.track.stopped, false);
  stream.getTracks().forEach(track => track.stop());
});

test('stops permission probe and retains device diagnostics when selection fails', async () => {
  const f = fixture({listed: [device('default', 'Default')]});
  const diagnostics = {};
  await assert.rejects(openTestMicrophone(f.mediaDevices, diagnostics), /Expected exactly one/);
  assert.equal(f.probe.track.stopped, true);
  assert.equal(f.calls.length, 1);
  assert.deepEqual(diagnostics.devices, [device('default', 'Default')]);
});

test('does not retry a rejected exact device request with a default device', async () => {
  const failure = new Error('requested device disappeared');
  const f = fixture({failure});
  await assert.rejects(openTestMicrophone(f.mediaDevices, {}), error => error === failure);
  assert.equal(f.probe.track.stopped, true);
  assert.equal(f.calls.length, 2);
});

test('rejects wrong captured ID and stops that stream', async () => {
  const f = fixture({capturedID: 'default'});
  const diagnostics = {};
  await assert.rejects(openTestMicrophone(f.mediaDevices, diagnostics), /Captured microphone ID does not match/);
  assert.equal(f.captured.track.stopped, true);
  assert.equal(diagnostics.captured.settings.deviceId, 'default');
});

test('permission and enumeration failures propagate with probe cleanup', async () => {
  const failure = new Error('permission denied');
  await assert.rejects(openTestMicrophone({getUserMedia: async () => { throw failure; }}, {}), error => error === failure);
  const f = fixture();
  f.mediaDevices.enumerateDevices = async () => { throw new Error('enumeration failed'); };
  await assert.rejects(openTestMicrophone(f.mediaDevices, {}), /enumeration failed/);
  assert.equal(f.probe.track.stopped, true);
  assert.equal(f.calls.length, 1);
});
