import { afterEach, expect, it } from 'vitest'
import { createApp, h, nextTick, type Component } from 'vue'
import { Button } from './button'
import { Input } from './input'
import { Textarea } from './textarea'
import { Select, SelectTrigger } from './select'
import { Card, CardHeader, CardContent, CardFooter } from './card'
import AppButton from './AppButton.vue'
import AppInput from './AppInput.vue'
import AppCard from './AppCard.vue'
import FormInput from '../form/FormInput.vue'
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from './dropdown-menu'

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => apps.splice(0).forEach((app) => app.unmount()))
function mount(
  component: Component,
  props: Record<string, unknown> = {},
  slots: Record<string, () => unknown> = { default: () => 'Synthetic' },
) {
  const root = document.createElement('div')
  const app = createApp({ render: () => h(component, props, slots) })
  apps.push(app)
  app.mount(root)
  return root
}

it.each([
  ['xs', '--goadmin-control-height-xs,1.75rem'],
  ['sm', '--goadmin-control-height-sm,2rem'],
  ['md', '--goadmin-control-height,2.25rem'],
  ['lg', '--goadmin-control-height-lg,2.5rem'],
  ['xl', '--goadmin-control-height-xl,2.75rem'],
  ['icon', '--goadmin-control-height,2.25rem'],
  ['icon-sm', '--goadmin-control-height-sm,2rem'],
  ['icon-lg', '--goadmin-control-height-lg,2.5rem'],
])('AppButton %s forwards its size token without a conflicting wrapper height', (size, token) => {
  const button = mount(AppButton, { size }).querySelector('button')!
  expect(button.className).toContain(token)
  const dimensions = [...button.classList].filter((value) => /^(h|size)-/.test(value))
  expect(dimensions).toHaveLength(1)
})

it.each([Button, AppButton, Input, AppInput, Textarea, AppCard])(
  'preserves explicit dimensions/radius overrides through %s',
  (component) => {
    const root = mount(component, { class: 'h-16 min-h-24 rounded-none' })
    const element = root.firstElementChild!
    expect(element.classList.contains('h-16')).toBe(true)
    expect(element.classList.contains('min-h-24')).toBe(true)
    expect(element.classList.contains('rounded-none')).toBe(true)
    expect(element.className).not.toContain('--goadmin-control-radius')
    expect(element.className).not.toContain('--goadmin-card-radius')
  },
)

it('keeps pill buttons and lets an explicit radius override the pill', () => {
  expect(mount(AppButton, { rounded: true }).querySelector('button')?.className).toContain(
    'rounded-full',
  )
  const button = mount(AppButton, {
    rounded: true,
    class: 'rounded-none',
    size: 'xl',
  }).querySelector('button')!
  expect(button.className).toContain('rounded-none')
  expect(button.className).not.toContain('rounded-full')
})

it.each(['sm', 'md', 'lg'])(
  'aligns FormInput %s icon/loading rows to the input size token',
  (size) => {
    const root = mount(
      FormInput,
      { size, loading: true, modelValue: 'Synthetic', id: 'synthetic-input' },
      { leftIcon: () => h('span', 'L'), rightIcon: () => h('button', 'R') },
    )
    const input = root.querySelector('input')!
    const height = [...input.classList].find((value) => value.startsWith('h-'))!
    expect(height).toContain('--goadmin-control-height')
    const overlays = root.querySelectorAll('div.absolute')
    expect(overlays).toHaveLength(3)
    overlays.forEach((overlay) => expect(overlay.classList.contains(height)).toBe(true))
  },
)

it('keeps FormInput explicit height/radius and error/readonly/disabled semantics', () => {
  const input = mount(FormInput, {
    class: 'h-16 rounded-none',
    error: 'Synthetic error',
    readonly: true,
    disabled: true,
    id: 'synthetic-state',
  }).querySelector('input')!
  expect(input.className).toContain('h-16')
  expect(input.className).toContain('rounded-none')
  expect(input.getAttribute('aria-invalid')).toBe('true')
  expect(input.getAttribute('aria-describedby')).toBe('synthetic-state-error')
  expect(input.readOnly).toBe(true)
  expect(input.disabled).toBe(true)
})

it('keeps Card/AppCard radius fallbacks and content/footer top padding exclusions', () => {
  for (const component of [Card, AppCard]) {
    expect(mount(component).firstElementChild?.className).toContain(
      '--goadmin-card-radius,var(--radius-xl,0.75rem)',
    )
  }
  expect(mount(CardHeader).firstElementChild?.className).toContain(
    'p-[var(--goadmin-card-padding,1.5rem)]',
  )
  for (const component of [CardContent, CardFooter]) {
    const element = mount(component).firstElementChild!
    expect(element.classList.contains('pt-0')).toBe(true)
    expect(element.className).toContain('--goadmin-card-padding,1.5rem')
    const override = mount(component, { class: 'p-2 pt-3' }).firstElementChild!
    expect(override.className).not.toContain('--goadmin-card-padding')
    expect(override.classList.contains('pt-0')).toBe(false)
    expect(override.classList.contains('p-2')).toBe(true)
    expect(override.classList.contains('pt-3')).toBe(true)
  }
})

it('forwards SelectTrigger control tokens while preserving explicit overrides', () => {
  const root = mount(
    Select,
    {},
    { default: () => h(SelectTrigger, { class: 'h-16 rounded-none' }, () => 'Synthetic') },
  )
  const trigger = root.querySelector('[role="combobox"]')!
  expect(trigger.className).toContain('h-16')
  expect(trigger.className).toContain('rounded-none')
  expect(trigger.className).not.toContain('--goadmin-control-height')
  expect(trigger.className).not.toContain('--goadmin-control-radius')
})

it('exposes stable hooks on shared controls and card sections', () => {
  for (const [component, family, part] of [
    [Button, 'button', 'root'],
    [AppButton, 'button', 'root'],
    [Input, 'input', 'root'],
    [AppInput, 'input', 'root'],
    [Textarea, 'textarea', 'root'],
    [Card, 'card', 'root'],
    [AppCard, 'card', 'root'],
    [CardHeader, 'card', 'header'],
    [CardContent, 'card', 'content'],
    [CardFooter, 'card', 'footer'],
  ] as const) {
    const element = mount(component).firstElementChild!
    expect(element.getAttribute('data-ui')).toBe(family)
    expect(element.getAttribute('data-part')).toBe(part)
  }
})

it('AppCard retains Card semantic tokens and yields each color to explicit host classes', () => {
  const card = mount(AppCard).firstElementChild!
  for (const token of ['bg', 'text', 'border']) {
    expect(card.className).toContain(`--goadmin-card-${token}`)
  }
  const override = mount(AppCard, {
    class: 'bg-background text-error border-transparent',
  }).firstElementChild!
  for (const token of ['bg', 'text', 'border']) {
    expect(override.className).not.toContain(`--goadmin-card-${token}`)
  }
  for (const name of ['bg-background', 'text-error', 'border-transparent']) {
    expect(override.classList.contains(name)).toBe(true)
  }
})

it.each([undefined, 'bg-background text-error border-transparent'])(
  'portal menu colors use mergeable recipes without inline bypass: %s',
  async (className) => {
    mount(
      DropdownMenu,
      { defaultOpen: true },
      {
        default: () => [
          h(DropdownMenuTrigger, {}, () => 'Open'),
          h(DropdownMenuContent, { class: className }, () => 'Menu'),
        ],
      },
    )
    await nextTick()
    await nextTick()
    const content = document.body.querySelector<HTMLElement>(
      '[data-ui="dropdown-menu"][data-part="content"]',
    )!
    expect(content).not.toBeNull()
    expect(content.style.backgroundColor).toBe('')
    expect(content.style.color).toBe('')
    if (className) {
      expect(content.className).not.toContain('--goadmin-menu-bg')
      expect(content.className).not.toContain('--goadmin-menu-text')
      expect(content.className).not.toContain('--goadmin-menu-border')
      for (const name of className.split(' ')) expect(content.classList.contains(name)).toBe(true)
    } else {
      expect(content.className).toContain('--goadmin-menu-bg,hsl(var(--popover))')
      expect(content.className).toContain('--goadmin-menu-text,hsl(var(--popover-foreground))')
    }
  },
)
