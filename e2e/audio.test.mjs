import { before, after, test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile, writeFile, mkdir } from 'node:fs/promises';
import { execFile, spawn } from 'node:child_process';
import { promisify } from 'node:util';
import { once } from 'node:events';
import { resolve } from 'node:path';
import { chromium } from 'playwright-core';
import { encodeWav, markerSamples, rawToFloat, segments, rms } from './audio-math.mjs';

const exec = promisify(execFile);
const artifacts = resolve(process.env.ARTIFACT_DIR || '../artifacts/audio');
const binary = resolve(process.env.TTS2MIC_BIN || '../bin/tts2mic-mcp');
let browser, server, baseURL;

before(async () => {
  await mkdir(artifacts, {recursive: true});
  const html = await readFile(new URL('./harness.html', import.meta.url));
  const output = encodeWav(markerSamples(48000), 48000);
  server = createServer((req, res) => {
    res.setHeader('Content-Type', req.url === '/output.wav' ? 'audio/wav' : 'text/html');
    res.end(req.url === '/output.wav' ? output : html);
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  baseURL = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({
    channel: 'chromium', headless: true, ignoreDefaultArgs: ['--mute-audio'],
    env: {...process.env, PULSE_SOURCE: 'tts2mic_mic', PULSE_SINK: 'tts2mic_rx'},
  });
}, {timeout: 30000});

after(async () => { await browser?.close(); if (server) await new Promise(resolve => server.close(resolve)); });

async function captureTest(name, body) {
  const context = await browser.newContext({permissions: ['microphone']});
  await context.tracing.start({screenshots: true, snapshots: true});
  const page = await context.newPage();
  try {
    await page.goto(baseURL);
    await page.click('#start');
    await page.waitForFunction(() => window.ready || window.problem);
    assert.equal(await page.evaluate(() => window.problem), undefined);
    assert.match(await page.evaluate(() => window.inputLabel), /TTS2Mic/i, 'browser captured the wrong device');
    await page.evaluate(() => { window.mic.samples = []; window.mic.recording = true; });
    await body(page);
  } finally {
    const mic = await page.evaluate(() => window.mic).catch(() => null);
    if (mic?.rate) await writeFile(resolve(artifacts, `${name}-mic.wav`), encodeWav(mic.samples, mic.rate));
    await context.tracing.stop({path: resolve(artifacts, `${name}-trace.zip`)});
    await context.close();
  }
}

function assertMarkers(samples, rate) {
  const groups = segments(samples, rate);
  assert.equal(groups.length, 2, `expected two complete markers: ${JSON.stringify(groups)}`);
  for (let i = 0; i < groups.length; i++) {
    assert.ok(Math.abs(groups[i].duration - 0.6) < 0.15, `wrong speed or missing tail: ${JSON.stringify(groups)}`);
    assert.ok(Math.abs(groups[i].hz - [440, 880][i]) < 20, `wrong pitch: ${JSON.stringify(groups)}`);
  }
}

for (const rate of [16000, 24000, 48000]) {
  test(`real microphone capture preserves ${rate} Hz WAV markers after reacquisition`, {timeout: 30000}, async () => {
    const channels = rate === 48000 ? 2 : 1;
    const file = resolve(artifacts, `input-${rate}.wav`);
    await writeFile(file, encodeWav(markerSamples(rate, channels), rate, channels));
    await captureTest(`input-${rate}`, async page => {
      await exec(binary, ['play', '--target', 'pipewire', '--file', file], {timeout: 15000});
      const afterDrain = await page.evaluate(() => window.mic.samples.length);
      await page.waitForFunction(count => window.mic.samples.length >= count + window.mic.rate * 0.5, afterDrain);
      const mic = await page.evaluate(() => window.mic);
      assertMarkers(mic.samples, mic.rate);
    });
  });
}

function recorder() {
  const child = spawn('parec', ['--device=tts2mic_rx.monitor', '--raw', '--format=s16le', '--rate=48000', '--channels=1']);
  const chunks = []; let size = 0, problem = '';
  child.stdout.on('data', chunk => { chunks.push(chunk); size += chunk.length; });
  child.stderr.on('data', chunk => { problem += chunk; });
  child.on('error', error => { problem += error.message; });
  const closed = new Promise(resolve => child.once('close', resolve));
  return {child, chunks, closed, size: () => size, problem: () => problem};
}

async function until(predicate, description) {
  const end = Date.now() + 5000;
  while (!predicate()) {
    if (Date.now() > end) throw new Error(`Timed out: ${description}`);
    await new Promise(resolve => setTimeout(resolve, 25));
  }
}

test('browser playback is recorded without feeding the microphone', {timeout: 30000}, async () => {
  await captureTest('output-isolation', async page => {
    const rec = recorder();
    try {
      await until(() => rec.size() >= 9600, `recorder readiness: ${rec.problem()}`);
      const baseline = rec.size();
      await page.click('#play');
      await page.waitForFunction(() => window.outputEnded || window.problem);
      assert.equal(await page.evaluate(() => window.problem), undefined);
      await until(() => rec.size() >= baseline + 48000 * 2 * 2.5, 'output capture tail');
      await page.waitForFunction(() => window.mic.samples.length >= window.mic.rate * 3);
      const mic = await page.evaluate(() => window.mic);
      assert.ok(rms(mic.samples) < 0.01, 'browser output leaked into synthetic microphone');
    } finally {
      rec.child.kill('SIGINT');
      const killer = setTimeout(() => rec.child.kill('SIGKILL'), 2000);
      await rec.closed; clearTimeout(killer);
      const raw = Buffer.concat(rec.chunks);
      await writeFile(resolve(artifacts, 'browser-output.wav'), encodeWav(rawToFloat(raw), 48000));
    }
    assertMarkers(rawToFloat(Buffer.concat(rec.chunks)), 48000);
  });
});
