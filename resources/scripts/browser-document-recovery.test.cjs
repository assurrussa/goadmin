const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const script = readFileSync(path.join(__dirname, '../../infrastructure/core/middlewares/browser-recovery.js'), 'utf8');

function response(status, code, extras = {}, headers = {}) {
  return {status, redirected: false, headers: {get: name => ({'x-goadmin-auth-recovery': '1', 'content-type': 'application/json', ...headers})[name]},
    json: async () => ({status, code, ...extras})};
}
async function run(responses) {
  const elements = Object.fromEntries(['status', 'retry', 'continue'].map(name => [name, {hidden: true, textContent: '', listeners: {}, addEventListener(type, callback) {this.listeners[type] = callback;}}]));
  const requests = [];
  let reloads = 0;
  let timer;
  let cleared = false;
  vm.runInNewContext(script, {
    document: {getElementById: id => elements[id.replace('auth-recovery-', '')]},
    location: {reload: () => reloads++}, AbortController,
    setTimeout: (callback, ms) => {assert.equal(ms, 30000); timer = callback; return 7;},
    clearTimeout: id => {assert.equal(id, 7); cleared = true;},
    fetch: async (url, init) => {requests.push({url, init}); const next = responses.shift(); if (next instanceof Error) throw next; return typeof next === 'function' ? next(init) : next;}
  });
  const settle = async () => {for (let i = 0; i < 20; i++) await Promise.resolve();};
  await settle();
  return {elements, requests, settle, timeout: () => timer(), reloads: () => reloads, cleared: () => cleared};
}

test('confirmed recovery requires explicit safe document continuation, never mutation replay', async () => {
  const result = await run([response(200, 'authentication_recovered', {csrfReady: true})]);
  assert.equal(result.elements.continue.hidden, false);
  assert.equal(result.reloads(), 0);
  assert.equal(result.requests.length, 1);
  const {url, init} = result.requests[0];
  assert.equal(url, '/auth/session/recover');
  for (const [key, value] of Object.entries({method: 'GET', mode: 'same-origin', credentials: 'same-origin', redirect: 'error', cache: 'no-store'})) assert.equal(init[key], value);
  assert.equal(init.headers['X-Goadmin-Auth-Recovery'], '1');
  assert.equal(init.headers.Accept, 'application/json');
  assert.equal(init.body, undefined);
  result.elements.continue.listeners.click();
  assert.equal(result.reloads(), 1);
  assert.equal(result.cleared(), true);
});

test('native cookie continuation is bounded by existing two-GET contract', async () => {
  const result = await run([response(409, 'authentication_recovery_continue'), response(409, 'authentication_recovery_continue')]);
  assert.equal(result.requests.length, 2);
  assert.equal(result.elements.retry.hidden, false);
  assert.equal(result.elements.continue.hidden, true);
  assert.equal(result.reloads(), 0);
});

test('409 then confirmed success shows continue', async () => {
  const result = await run([response(409, 'authentication_recovery_continue'), response(200, 'authentication_recovered', {csrfReady: true})]);
  assert.equal(result.requests.length, 2);
  assert.equal(result.elements.continue.hidden, false);
});

test('denial, malformed success, foreign contract and unknown network outcome never navigate', async () => {
  for (const rejected of [response(403, 'sso_admission_denied'), response(200, 'authentication_recovered'), response(200, 'authentication_recovered', {csrfReady: true}, {'x-goadmin-auth-recovery': '0'}), new Error('unknown outcome')]) {
    const result = await run([rejected]);
    assert.equal(result.elements.retry.hidden, false);
    assert.equal(result.elements.continue.hidden, true);
    assert.equal(result.reloads(), 0);
    assert.equal(result.requests.length, 1);
  }
});

test('failure permits only explicit new bounded attempt', async () => {
  const result = await run([response(403, 'denied'), response(200, 'authentication_recovered', {csrfReady: true})]);
  assert.equal(result.requests.length, 1);
  await result.elements.retry.listeners.click();
  await result.settle();
  assert.equal(result.requests.length, 2);
  assert.equal(result.elements.continue.hidden, false);
  assert.equal(result.reloads(), 0);
});

test('timeout aborts the bounded attempt and concurrent clicks do not overlap', async () => {
  const result = await run([init => new Promise((resolve, reject) => init.signal.addEventListener('abort', () => reject(new Error('timeout')), {once: true}))]);
  assert.equal(result.requests.length, 1);
  await result.elements.retry.listeners.click();
  assert.equal(result.requests.length, 1);
  result.timeout();
  await result.settle();
  assert.equal(result.requests[0].init.signal.aborted, true);
  assert.equal(result.elements.retry.hidden, false);
  assert.equal(result.elements.continue.hidden, true);
  assert.equal(result.cleared(), true);
});

test('script route is registered before static files and auth middleware', () => {
  const source = readFileSync(path.join(__dirname, '../../bootstrap/server.go'), 'utf8');
  const start = source.indexOf('func registerStaticRoutes(');
  const route = source.indexOf('fiberApp.Get("/public/auth-recovery.js"', start);
  assert.ok(route > start);
  assert.ok(route < source.indexOf('if publicFS != nil', start));
  assert.ok(route < source.indexOf('for _, middleware := range adminMiddlewares', start));
});
