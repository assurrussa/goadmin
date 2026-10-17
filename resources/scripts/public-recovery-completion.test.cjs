const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const { test } = require('node:test')
const { JSDOM } = require('jsdom')
const source = fs.readFileSync(path.join(__dirname, '../../host/assisted_recovery.go'), 'utf8')
const script = source.match(/const recoveryCompletionScript = `([\s\S]*?)`/)[1]

for (const statusCode of [200, 400, 503, 'network']) {
  test(`completion ${statusCode} outcome changes RU → EN → RU without another POST`, async () => {
    const dom = new JSDOM(
      `<h1>Завершение восстановления доступа</h1>
      <form action="/auth/assisted-recovery"><input name=csrf value=synthetic-csrf>
      <input name=token value=synthetic-token><input name=password value=synthetic-password>
      <input name=password_confirmation value=synthetic-password><input name=code value=synthetic-code>
      <button type=submit>Продолжить</button><p role=status></p></form>
      <button type=button id=language>English / Русский</button>`,
      {
        url: 'https://admin.example.test/auth/assisted-recovery',
        runScripts: 'outside-only',
      },
    )
    try {
      let requests = 0
      dom.window.localStorage.setItem('goadmin.locale', 'ru')
      dom.window.fetch = async () => {
        requests++
        if (statusCode === 'network') throw new Error('response lost')
        return { status: statusCode, ok: statusCode >= 200 && statusCode < 300 }
      }
      dom.window.eval(script)
      const form = dom.window.document.querySelector('form')
      form.dispatchEvent(new dom.window.Event('submit', { cancelable: true }))
      await new Promise(setImmediate)
      const result = dom.window.document.querySelector('[role=status]')
      const ru = result.textContent
      assert.match(
        ru,
        statusCode === 200
          ? /Доступ восстановлен/
          : statusCode === 400
            ? /Запрос отклонён/
            : /Результат неизвестен/,
      )
      dom.window.document.querySelector('#language').click()
      assert.match(
        result.textContent,
        statusCode === 200
          ? /Recovery completed/
          : statusCode === 400
            ? /Recovery was rejected/
            : /Outcome unknown/,
      )
      dom.window.document.querySelector('#language').click()
      assert.equal(result.textContent, ru)
      assert.equal(form.querySelector('button').disabled, true)
      for (const name of ['token', 'password', 'password_confirmation', 'code']) {
        assert.equal(form.elements[name].value, '')
      }
      form.dispatchEvent(new dom.window.Event('submit', { cancelable: true }))
      await new Promise(setImmediate)
      assert.equal(requests, 1)
    } finally {
      dom.window.close()
    }
  })
}
