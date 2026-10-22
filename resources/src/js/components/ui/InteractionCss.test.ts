// @vitest-environment node
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import postcss, { type Rule } from 'postcss'
import tailwind from '@tailwindcss/postcss'
import { expect, it } from 'vitest'

it('compiles native/portal interaction defaults in base, leaving host utilities in control', async () => {
  const from = fileURLToPath(new URL('../../../css/app.css', import.meta.url))
  const result = await postcss([tailwind()]).process(readFileSync(from, 'utf8'), { from })
  const rules: Rule[] = []
  result.root.walkRules((rule) => {
    rule.selector = rule.selector
      .replace(/\s+/g, ' ')
      .replace(/\(\s+/g, '(')
      .replace(/\s+\)/g, ')')
      .replace(/"/g, "'")
    rules.push(rule)
  })
  const pointer = rules.find(
    (rule) =>
      rule.selector.startsWith(':where(button, a[href]') &&
      rule.nodes.some(
        (node) => node.type === 'decl' && node.prop === 'cursor' && node.value === 'pointer',
      ),
  )!
  expect(pointer).toBeDefined()
  expect(pointer.selector).toContain("[role='option']")
  expect(pointer.selector).toContain("[role^='menuitem']")
  expect(pointer.selector).not.toContain('#app')
  expect(pointer.parent?.type).toBe('atrule')
  expect(pointer.parent && 'params' in pointer.parent ? pointer.parent.params : '').toBe('base')
  const disabled = rules.find(
    (rule) => rule.selector === ":where(:disabled, [aria-disabled='true'], [data-disabled])",
  )!
  expect(
    disabled.nodes.some(
      (node) => node.type === 'decl' && node.prop === 'cursor' && node.value === 'not-allowed',
    ),
  ).toBe(true)
  const text = rules.find((rule) => rule.selector.startsWith(':where(input:not([type])'))!
  expect(
    text.nodes.some(
      (node) => node.type === 'decl' && node.prop === 'cursor' && node.value === 'text',
    ),
  ).toBe(true)
  const focus = rules.find(
    (rule) =>
      rule.selector.startsWith(':where(button, a[href]') &&
      rule.selector.endsWith(':focus-visible'),
  )!
  expect(
    focus.nodes.some(
      (node) =>
        node.type === 'decl' && node.prop === 'outline' && node.value === 'var(--outline-focus)',
    ),
  ).toBe(true)
  for (const state of ['hover', 'active']) {
    expect(
      rules.some(
        (rule) =>
          rule.selector.includes(":not(:disabled, [aria-disabled='true'], [data-disabled])") &&
          rule.selector.endsWith(`:${state}`),
      ),
    ).toBe(true)
  }
  expect(result.css).toContain('--goadmin-dialog-bg')
  expect(result.css).toContain('--goadmin-menu-bg')
  expect(result.css).toContain('--goadmin-card-bg')
  const declarations: string[] = []
  result.root.walkDecls((declaration) => {
    declarations.push(`${declaration.prop}:${declaration.value.replace(/\s+/g, '')}`)
  })
  expect(declarations).toContain('background-color:var(--goadmin-menu-bg,hsl(var(--popover)))')
  expect(declarations).toContain('color:var(--goadmin-menu-text,hsl(var(--popover-foreground)))')
  expect(declarations).toContain('background-color:var(--goadmin-card-bg,var(--color-card))')
  const mediumShadow = '0 4px 6px -1px rgb(0 0 0/0.1),0 2px 4px -2px rgb(0 0 0/0.1)'
  const largeShadow = '0 10px 15px -3px rgb(0 0 0/0.1),0 4px 6px -4px rgb(0 0 0/0.1)'
  for (const [token, shadow] of [
    ['menu', mediumShadow],
    ['menu', largeShadow],
    ['dialog', largeShadow],
  ]) {
    expect(declarations).toContain(
      `--tw-shadow:var(--goadmin-${token}-shadow,${shadow.replace(/\s+/g, '')})`,
    )
  }
}, 20000)
