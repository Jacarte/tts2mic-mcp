// Deterministic PCM fixtures and signal checks; no speech provider is involved.
export function encodeWav(samples, rate, channels = 1) {
  const out = Buffer.alloc(44 + samples.length * 2);
  out.write('RIFF'); out.writeUInt32LE(out.length - 8, 4); out.write('WAVEfmt ', 8);
  out.writeUInt32LE(16, 16); out.writeUInt16LE(1, 20); out.writeUInt16LE(channels, 22);
  out.writeUInt32LE(rate, 24); out.writeUInt32LE(rate * channels * 2, 28);
  out.writeUInt16LE(channels * 2, 32); out.writeUInt16LE(16, 34);
  out.write('data', 36); out.writeUInt32LE(samples.length * 2, 40);
  samples.forEach((sample, index) => out.writeInt16LE(Math.round(Math.max(-1, Math.min(1, sample)) * 32767), 44 + index * 2));
  return out;
}

export function markerSamples(rate, channels = 1) {
  const samples = new Float32Array(Math.round(rate * 1.6) * channels);
  for (let frame = 0; frame < samples.length / channels; frame++) {
    const t = frame / rate;
    const hz = t >= 0.1 && t < 0.7 ? 440 : t >= 0.9 && t < 1.5 ? 880 : 0;
    for (let ch = 0; ch < channels; ch++) samples[frame * channels + ch] = hz ? 0.3 * Math.sin(2 * Math.PI * hz * t) : 0;
  }
  return samples;
}

export function rawToFloat(raw) {
  const out = new Float32Array(Math.floor(raw.length / 2));
  for (let i = 0; i < out.length; i++) out[i] = raw.readInt16LE(i * 2) / 32768;
  return out;
}

export function rms(samples) {
  return Math.sqrt(samples.reduce((sum, value) => sum + value * value, 0) / Math.max(1, samples.length));
}

export function segments(samples, rate) {
  const block = Math.round(rate / 100); // 10 ms windows.
  const groups = [];
  let start;
  for (let i = 0; i <= samples.length; i += block) {
    const active = i < samples.length && rms(samples.slice(i, i + block)) > 0.04;
    if (active && start === undefined) start = i;
    if (!active && start !== undefined) {
      const middle = samples.slice(start + Math.round(rate * 0.05), i - Math.round(rate * 0.05));
      let crossings = 0;
      for (let j = 1; j < middle.length; j++) if (middle[j - 1] <= 0 && middle[j] > 0) crossings++;
      groups.push({ duration: (i - start) / rate, hz: crossings * rate / Math.max(1, middle.length) });
      start = undefined;
    }
  }
  return groups.filter(group => group.duration > 0.1);
}
