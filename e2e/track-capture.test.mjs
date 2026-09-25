import {test} from 'node:test';
import assert from 'node:assert/strict';
import {appendFrame, captureFrames, createCapture} from './track-capture.mjs';

function frame(samples, timestamp = 0, rate = 44100) {
  return {
    sampleRate: rate, numberOfFrames: samples.length, numberOfChannels: 1,
    timestamp, closed: false,
    copyTo(out, options) {
      assert.deepEqual(options, {planeIndex: 0, format: 'f32-planar'});
      out.set(samples);
    },
    close() { this.closed = true; },
  };
}

test('raw capture retains a complete zero-valued 1024-sample block and source timestamps', () => {
  const capture = createCapture(); capture.recording = true;
  const first = frame([0.25, 0.5], 1000);
  const gap = frame(new Array(1024).fill(0), 1045);
  const last = frame([-0.5, -0.25], 24265);
  for (const f of [first, gap, last]) appendFrame(capture, f);
  assert.deepEqual(capture.samples, [0.25, 0.5, ...new Array(1024).fill(0), -0.5, -0.25]);
  assert.deepEqual(capture.frames, [
    {timestamp: 1000, offset: 0, length: 2},
    {timestamp: 1045, offset: 2, length: 1024},
    {timestamp: 24265, offset: 1026, length: 2},
  ]);
  assert.ok([first, gap, last].every(f => f.closed));
});

test('arming discards no recorded audio and sample rates are not relabelled', () => {
  for (const rate of [16000, 24000, 44100, 48000]) {
    const capture = createCapture();
    const warmup = frame([1, 1], 0, rate); appendFrame(capture, warmup);
    assert.ok(warmup.closed); assert.equal(capture.samples.length, 0);
    capture.recording = true;
    appendFrame(capture, frame([0.25, -0.25], 1000, rate));
    assert.equal(capture.rate, rate);
    assert.deepEqual(capture.samples, [0.25, -0.25]);
  }
});

test('format changes, copy failures and overflow fail explicitly and close AudioData', () => {
  const capture = createCapture(); capture.recording = true;
  appendFrame(capture, frame([0.5]));
  const changed = frame([1], 1000, 48000);
  assert.throws(() => appendFrame(capture, changed), /format changed/);
  assert.ok(changed.closed);
  const broken = frame([1]); broken.copyTo = () => { throw new Error('copy failed'); };
  assert.throws(() => appendFrame(capture, broken), /copy failed/);
  assert.ok(broken.closed);
  capture.samples.length = 20 * capture.rate;
  const overflow = frame([1]);
  assert.throws(() => appendFrame(capture, overflow), /20-second limit/);
  assert.ok(overflow.closed);
});

test('reader consumes every frame, reports readiness and releases its lock', async () => {
  const capture = createCapture(); capture.recording = true;
  const frames = [frame([0.25]), frame([0]), frame([-0.25])];
  let index = 0, ready = 0, released = false;
  await captureFrames({
    async read() { return index < frames.length ? {value: frames[index++], done: false} : {done: true}; },
    releaseLock() { released = true; },
  }, capture, () => ready++);
  assert.deepEqual(capture.samples, [0.25, 0, -0.25]);
  assert.equal(ready, 3); assert.ok(released);
});

test('reader failure propagates and releases the lock instead of producing a partial success', async () => {
  let released = false;
  await assert.rejects(captureFrames({
    async read() { throw new Error('capture disconnected'); },
    releaseLock() { released = true; },
  }, createCapture(), () => {}), /capture disconnected/);
  assert.ok(released);
});
