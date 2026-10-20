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
async function run(responses, storage = new Map()) {
  const elements = Object.fromEntries(['status', 'retry', 'continue', 'title', 'sign-in'].map(name => [name, {hidden: true, textContent: '', listeners: {}, addEventListener(type, callback) {this.listeners[type] = callback;}}]));
  const requests = [];
  let reloads = 0;
  let timer;
  let cleared = false;
  vm.runInNewContext(script, {
    document: {getElementById: id => elements[id.replace('auth-recovery-', '')]},
    location: {reload: () => reloads++}, AbortController,
    window: {addEventListener() {}},
    sessionStorage: {getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value)},
    setTimeout: (callback, ms) => {assert.equal(ms, 30000); timer = callback; return 7;},
    clearTimeout: id => {assert.equal(id, 7); cleared = true;},
    fetch: async (url, init) => {requests.push({url, init}); const next = responses.shift(); if (next instanceof Error) throw next; return typeof next === 'function' ? next(init) : next;}
  });
  const settle = async () => {for (let i = 0; i < 20; i++) await Promise.resolve();};
  await settle();
  return {elements, requests, settle, timeout: () => timer(), reloads: () => reloads, cleared: () => cleared};
}

test('confirmed recovery automatically returns with a safe GET, never mutation replay', async () => {
  const result = await run([response(200, 'authentication_recovered', {csrfReady: true})]);
  assert.equal(result.elements.continue.hidden, false);
  assert.equal(result.reloads(), 1);
  assert.equal(result.requests.length, 1);
  const {url, init} = result.requests[0];
  assert.equal(url, '/auth/session/recover');
  for (const [key, value] of Object.entries({method: 'GET', mode: 'same-origin', credentials: 'same-origin', redirect: 'error', cache: 'no-store'})) assert.equal(init[key], value);
  assert.equal(init.headers['X-Goadmin-Auth-Recovery'], '1');
  assert.equal(init.headers.Accept, 'application/json');
  assert.equal(init.body, undefined);
  result.elements.continue.listeners.click();
  assert.equal(result.reloads(), 2);
  assert.equal(result.cleared(), true);
});

test('native cookie continuation is bounded by existing two-GET contract', async () => {
  const result = await run([response(409, 'authentication_recovery_continue'), response(409, 'authentication_recovery_continue')]);
  assert.equal(result.requests.length, 2);
  assert.equal(result.elements.retry.hidden, false);
  assert.equal(result.elements.continue.hidden, true);
  assert.equal(result.reloads(), 0);
});

test('409 then confirmed success returns automatically and retains fallback', async () => {
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
  assert.equal(result.reloads(), 1);
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


test('return marker survives document recreation and stops automatic loops', async () => {
  const storage = new Map();
  const first = await run([response(200, 'authentication_recovered', {csrfReady: true})], storage);
  assert.equal(first.reloads(), 1);
  const second = await run([response(200, 'authentication_recovered', {csrfReady: true})], storage);
  assert.equal(second.requests.length, 0);
  assert.equal(second.reloads(), 0);
  assert.equal(second.elements.retry.hidden, false);
  await second.elements.retry.listeners.click();
  assert.equal(second.requests.length, 1);
  assert.equal(second.reloads(), 0);
  assert.equal(second.elements.continue.hidden, false);
  second.elements.continue.listeners.click();
  assert.equal(second.reloads(), 1);
});

test('unavailable storage safely falls back to manual recovery and continuation', async () => {
  const result = await run([response(200, 'authentication_recovered', {csrfReady: true})], {
    get() { throw new Error('storage unavailable'); }, set() { throw new Error('storage unavailable'); }
  });
  assert.equal(result.requests.length, 0);
  await result.elements.retry.listeners.click();
  assert.equal(result.requests.length, 1);
  assert.equal(result.reloads(), 0);
  assert.equal(result.elements.continue.hidden, false);
});

test('expired marker permits a later recovery without storing URL or identity', async () => {
  const storage = new Map([['goadmin:recovery:auto-return:v1', String(Date.now() - 61000)]]);
  const result = await run([response(200, 'authentication_recovered', {csrfReady: true})], storage);
  assert.equal(result.reloads(), 1);
  assert.equal(storage.size, 1);
  assert.match(storage.get('goadmin:recovery:auto-return:v1'), /^\d+$/);
  assert.equal(result.elements['sign-in'].hidden, true);
  assert.equal(result.elements.title.textContent, 'Возвращаемся на страницу');
});


test('choosing fresh sign-in invalidates an in-flight successful recovery', async () => {
  let resolve;
  const result = await run([() => new Promise(done => { resolve = done; })]);
  result.elements['sign-in'].listeners.click({button: 0});
  resolve(response(200, 'authentication_recovered', {csrfReady: true}));
  await result.settle();
  assert.equal(result.requests[0].init.signal.aborted, true);
  assert.equal(result.reloads(), 0);
  assert.equal(result.elements.continue.hidden, true);
});


test('modified sign-in click does not abandon the current tab recovery', async () => {
  for (const modifiers of [{ctrlKey: true}, {metaKey: true}, {shiftKey: true}]) {
    let resolve;
    const result = await run([() => new Promise(done => { resolve = done; })]);
    result.elements['sign-in'].listeners.click({button: 0, ...modifiers});
    resolve(response(200, 'authentication_recovered', {csrfReady: true}));
    await result.settle();
    assert.equal(result.requests[0].init.signal.aborted, false);
    assert.equal(result.reloads(), 1);
  }
});
