// Observe the actual microphone track, not audio re-clocked through a Web Audio
// destination. No resampling, silence removal, gap joining or tone filtering.
// MediaStreamTrackProcessor audio support is Chromium-specific: this is a pinned
// Chromium conformance observer, not a portable production capture API.
export function createCapture() {
  return {samples: [], recording: false, rate: 0, channels: 0, frames: []};
}

export function appendFrame(capture, frame) {
  try {
    const {sampleRate, numberOfFrames, numberOfChannels, timestamp} = frame;
    if (!Number.isFinite(sampleRate) || sampleRate <= 0 ||
        !Number.isInteger(numberOfFrames) || numberOfFrames <= 0 ||
        !Number.isInteger(numberOfChannels) || numberOfChannels <= 0) {
      throw new Error('Invalid microphone AudioData format');
    }
    if (capture.rate && (capture.rate !== sampleRate || capture.channels !== numberOfChannels)) {
      throw new Error('Microphone format changed during capture');
    }
    capture.rate = sampleRate;
    capture.channels = numberOfChannels;
    if (!capture.recording) return;
    if (capture.samples.length + numberOfFrames > 20 * sampleRate) {
      throw new Error('Microphone recording exceeded the 20-second limit');
    }
    const samples = new Float32Array(numberOfFrames);
    frame.copyTo(samples, {planeIndex: 0, format: 'f32-planar'});
    // Retain source timestamps and frame boundaries to expose dropped/delayed
    // frames in artifacts; never concatenate around zero-valued audio blocks.
    capture.frames.push({timestamp, offset: capture.samples.length, length: numberOfFrames});
    for (const sample of samples) capture.samples.push(sample);
  } finally {
    frame.close();
  }
}

export async function captureFrames(reader, capture, onReady) {
  try {
    for (;;) {
      const {value, done} = await reader.read();
      if (done) return;
      appendFrame(capture, value);
      onReady();
    }
  } finally {
    reader.releaseLock();
  }
}
