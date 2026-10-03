const path = require('node:path')
const test = require('node:test')
const assert = require('node:assert/strict')
const { ESLint } = require('eslint')

const eslint = new ESLint({ cwd: path.resolve(__dirname, '..') })

test('direct presets retain TypeScript syntax and unused-variable checks', async () => {
  for (const extension of ['ts', 'mts', 'tsx']) {
    const [valid] = await eslint.lintText('export const label: string = "admin"', {
      filePath: `src/LintFixture.${extension}`,
    })
    assert.equal(valid.errorCount, 0)
    const [invalid] = await eslint.lintText('const unused: string = "admin"', {
      filePath: `src/LintFixture.${extension}`,
    })
    assert(
      invalid.messages.some((message) => message.ruleId === '@typescript-eslint/no-unused-vars'),
    )
  }
})

test('Vue retains template TypeScript parsing, script setup bindings and language policy', async () => {
  const [valid] = await eslint.lintText(
    '<script setup lang="ts">const label: string = "admin"</script>\n<template><p>{{ (label as string).length }}</p></template>',
    { filePath: 'src/LintFixture.vue' },
  )
  assert.equal(valid.errorCount, 0, JSON.stringify(valid.messages))
  const [invalid] = await eslint.lintText(
    '<script setup>const label = "admin"</script><template>{{ label }}</template>',
    {
      filePath: 'src/LintFixture.vue',
    },
  )
  assert(invalid.messages.some((message) => message.ruleId === 'vue/block-lang'))
  const config = await eslint.calculateConfigForFile('src/LintFixture.vue')
  assert.equal(config.languageOptions.parser.meta.name, 'vue-eslint-parser')
  assert.equal(config.rules['vue/no-parsing-error'][0], 2)
  assert.equal(config.rules['@typescript-eslint/no-explicit-any'][0], 1)
  assert.equal(config.languageOptions.parserOptions.projectService, undefined)
})

test('test plugins, UI exceptions and global ignores remain scoped', async () => {
  const unit = await eslint.calculateConfigForFile('src/js/services/__tests__/fixture.test.ts')
  const e2e = await eslint.calculateConfigForFile('e2e/fixture.spec.ts')
  const normal = await eslint.calculateConfigForFile('src/js/app.ts')
  const ui = await eslint.calculateConfigForFile('src/js/components/ui/Fixture.vue')
  assert.equal(unit.rules['vitest/no-focused-tests'][0], 2)
  assert(e2e.rules['playwright/no-focused-test'][0] > 0)
  assert.equal(normal.rules['vitest/no-focused-tests'], undefined)
  assert.equal(normal.rules['playwright/no-focused-test'], undefined)
  assert.equal(ui.rules['vue/multi-word-component-names'][0], 0)
  for (const file of ['dist/file.ts', 'coverage/file.ts', 'scripts/helper.cjs']) {
    assert.equal(await eslint.isPathIgnored(file), true)
  }
})
