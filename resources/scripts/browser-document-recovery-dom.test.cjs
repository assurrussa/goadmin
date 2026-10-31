const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const path = require('node:path');
const {test} = require('node:test');
const {JSDOM} = require('jsdom');

const middleware = path.join(__dirname, '../../infrastructure/core/middlewares');
const source = readFileSync(path.join(middleware, 'browser-recovery.go'), 'utf8');
const template = source.match(/template\.New\("recovery"\)\.Parse\(`([\s\S]*?)`\)\)/)[1]
  .replace(/`\s*\+\s*`/g, '')
  .replace(/{{if \.}}|{{end}}/g, '')
  .replace('{{.}}', '/sso');
const styles = readFileSync(path.join(middleware, 'browser-recovery.css'), 'utf8');
const script = readFileSync(path.join(middleware, 'browser-recovery.js'), 'utf8');

test('rendered terminal recovery promotes only the trusted sign-in anchor', async () => {
  const dom = new JSDOM(template.replace('</head>', `<style>${styles}</style></head>`), {
    url: 'https://admin.example.test/protected', runScripts: 'outside-only',
  });
  try {
    const requests = [];
    dom.window.fetch = async (url, init) => {
      requests.push({url, init});
      return {status: 403, redirected: false, headers: {get: name => ({
        'x-goadmin-auth-recovery': '1', 'content-type': 'application/json',
      })[name]}, json: async () => ({status: 403, code: 'reauthentication_required'})};
    };
    dom.window.eval(script);
    for (let i = 0; i < 20; i++) await Promise.resolve();
    const document = dom.window.document;
    const signIn = document.getElementById('auth-recovery-sign-in');
    assert.equal(document.title, 'Нужно войти заново');
    assert.equal(signIn.tagName, 'A');
    assert.equal(signIn.getAttribute('href'), '/sso');
    assert.equal(signIn.hidden, false);
    assert.equal(signIn.classList.contains('primary-action'), true);
    assert.equal(dom.window.getComputedStyle(signIn).backgroundColor, 'rgb(37, 99, 235)');
    assert.equal(dom.window.getComputedStyle(signIn).color, 'rgb(255, 255, 255)');
    assert.equal(document.getElementById('auth-recovery-retry').hidden, true);
    assert.equal(document.getElementById('auth-recovery-continue').hidden, true);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].url, '/auth/session/recover');
    assert.equal(requests[0].init.method, 'GET');
    assert.equal(requests[0].init.body, undefined);
  } finally {
    dom.window.close();
  }
});
