import test from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

// Minimal protocol client, deliberately separate from the application's SDK.
async function connect() {
  const dir = await mkdtemp(join(tmpdir(), 'tts2mic-mcp-test-'));
  const binary = resolve(process.env.TTS2MIC_BIN || '../bin/tts2mic-mcp');
  const child = spawn(binary, [], {env: {...process.env,
    TTS_PROVIDER: 'stub', TTS_PROVIDER_NAME: 'stub',
    TTS_CACHE_DIR: join(dir, 'cache'), TTS2MIC_CLIP_DIR: join(dir, 'clips'),
    TTS2MIC_BACKEND: 'pipewire', TTS2MIC_PULSE_SINK: 'deliberately_missing_sink',
  }});
  const pending = new Map(); let counter = 0, stderr = '';
  child.stderr.on('data', chunk => { stderr += chunk; });
  const failPending = error => { for (const waiter of pending.values()) waiter.reject(error); pending.clear(); };
  child.on('error', failPending);
  const exit = new Promise(resolve => child.once('close', (code, signal) => {
    failPending(new Error(`MCP exited: ${code}/${signal}: ${stderr}`)); resolve({code, signal});
  }));
  const lines = createInterface({input: child.stdout});
  lines.on('line', line => {
    let message;
    try { message = JSON.parse(line); } catch { failPending(new Error(`Non-JSON MCP stdout: ${line}`)); return; }
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    if (message.error) waiter.reject(new Error(JSON.stringify(message.error))); else waiter.resolve(message.result);
  });
  async function request(method, params) {
    const id = ++counter;
    let timer;
    const response = new Promise((resolve, reject) => {
      pending.set(id, {resolve, reject});
      timer = setTimeout(() => { pending.delete(id); reject(new Error(`MCP timeout: ${method}: ${stderr}`)); }, 10000);
    });
    child.stdin.write(JSON.stringify({jsonrpc:'2.0', id, method, params}) + '\n');
    try { return await response; } finally { clearTimeout(timer); }
  }
  async function close() {
    child.stdin.end();
    const timer = setTimeout(() => child.kill('SIGKILL'), 3000);
    const status = await exit;
    clearTimeout(timer); lines.close(); await rm(dir, {recursive:true, force:true});
    assert.equal(status.code, 0, `MCP did not shut down cleanly: ${JSON.stringify(status)}: ${stderr}`);
  }
  try {
    await request('initialize', {protocolVersion:'2024-11-05', capabilities:{}, clientInfo:{name:'audio-ci', version:'1'}});
    child.stdin.write(JSON.stringify({jsonrpc:'2.0', method:'notifications/initialized'}) + '\n');
  } catch (error) { await close(); throw error; }
  return {
    request, close,
    async call(name, args) { return request('tools/call', {name, arguments:args}); },
  };
}

function value(result) {
  assert.ok(!result.isError, JSON.stringify(result));
  return result.structuredContent || JSON.parse(result.content[0].text);
}

async function terminal(client, id) {
  for (let i=0; i<100; i++) {
    const status = value(await client.call('job_status', {id}));
    if (['completed','failed','cancelled'].includes(status.state)) return status;
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  throw new Error('Job never reached a terminal state');
}

test('MCP reports backend failures, validates delay, cancels jobs, and cleans up on EOF', {timeout:20000}, async () => {
  const client = await connect();
  try {
    const listed = await client.request('tools/list', {});
    for (const name of ['speak','speak_delay','prepare','play','job_status','cancel_job']) {
      assert.ok(listed.tools.some(tool => tool.name === name), `missing tool: ${name}`);
    }
    const clip = value(await client.call('prepare', {text:'probe'}));
    assert.equal(clip.sample_rate, 16000);
    const job = value(await client.call('play', {clip_id:clip.clip_id}));
    const failed = await terminal(client, job.id);
    assert.equal(failed.state, 'failed');
    assert.match(failed.error, /paplay/);
    assert.ok((await client.call('speak_delay', {text:'probe', delay_ms:'-1s'})).isError);
    assert.ok((await client.call('speak_delay', {text:'probe', delay_ms:'500'})).isError);
    const delayed = value(await client.call('play', {clip_id:clip.clip_id, delay:'10s'}));
    value(await client.call('cancel_job', {id:delayed.id}));
    assert.equal((await terminal(client, delayed.id)).state, 'cancelled');
    // Closing stdin must cancel this job rather than leave a detached process.
    value(await client.call('play', {clip_id:clip.clip_id, delay:'10s'}));
  } finally { await client.close(); }
});
