import test from 'node:test';
import assert from 'node:assert/strict';
import { encodeWav, markerSamples, rawToFloat, segments } from './audio-math.mjs';

test('fixture analyzer detects both markers and catches incorrect sample rates', () => {
  for (const rate of [16000, 24000, 48000]) {
    const wav = encodeWav(markerSamples(rate), rate);
    const decoded = rawToFloat(wav.subarray(44));
    const groups = segments(decoded, rate);
    assert.equal(groups.length, 2);
    for (let i = 0; i < groups.length; i++) {
      assert.ok(Math.abs(groups[i].duration - 0.6) < 0.02);
      assert.ok(Math.abs(groups[i].hz - [440, 880][i]) < 5);
    }
  }
  const wrong = segments(markerSamples(24000), 16000);
  assert.ok(Math.abs(wrong[0].duration - 0.6) > 0.2);
  assert.ok(Math.abs(wrong[0].hz - 440) > 100);
});
