import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive, type Component } from 'vue'
import { Button } from './button'
import AppButton from './AppButton.vue'

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => apps.splice(0).forEach((app) => app.unmount()))
function mount(
  component: Component,
  props: Record<string, unknown>,
  child = () => h('span', 'Save'),
) {
  const root = document.createElement('div')
  const app = createApp({ render: () => h(component, props, { default: child }) })
  apps.push(app)
  app.mount(root)
  return root
}
function activation(element: Element) {
  const click = new MouseEvent('click', { bubbles: true, cancelable: true })
  element.dispatchEvent(click)
  const keys = ['Enter', ' '].flatMap((key) =>
    ['keydown', 'keyup'].map((type) => {
      const event = new KeyboardEvent(type, { key, bubbles: true, cancelable: true })
      element.dispatchEvent(event)
      return event
    }),
  )
  return [click, ...keys]
}
describe.each([Button, AppButton])('disabled activation contract', (component) => {
  it.each(['button', 'a'])('suppresses click and keyboard for as=%s', (as) => {
    const onClick = vi.fn()
    const onKeydown = vi.fn()
    const root = mount(component, { as, href: '#save', disabled: true, onClick, onKeydown })
    const element = root.firstElementChild!
    expect(element.getAttribute('aria-disabled')).toBe('true')
    expect(element.getAttribute('tabindex')).toBe('-1')
    expect(element.hasAttribute('disabled')).toBe(true)
    expect(activation(element).every((event) => event.defaultPrevented)).toBe(true)
    expect(onClick).not.toHaveBeenCalled()
    expect(onKeydown).not.toHaveBeenCalled()
  })
  it('fences disabled asChild links even with child tabindex and listeners', () => {
    const clicked = vi.fn()
    const keyed = vi.fn()
    const root = mount(component, { asChild: true, disabled: true }, () =>
      h('a', { href: '#save', tabindex: 0, onClick: clicked, onKeydown: keyed }, 'Save'),
    )
    const link = root.querySelector('a')!
    expect(link.getAttribute('tabindex')).toBe('-1')
    expect(activation(link).every((event) => event.defaultPrevented)).toBe(true)
    expect(clicked).not.toHaveBeenCalled()
    expect(keyed).not.toHaveBeenCalled()
  })
  it('preserves enabled asChild semantics, listeners, focus order and host classes', () => {
    const clicked = vi.fn()
    const root = mount(component, { asChild: true, class: 'h-16 rounded-none' }, () =>
      h('a', { href: '#save', tabindex: 2, onClick: clicked }, 'Save'),
    )
    const link = root.querySelector('a')!
    expect(link.getAttribute('tabindex')).toBe('2')
    expect(link.getAttribute('href')).toBe('#save')
    expect(link.getAttribute('aria-disabled')).toBeNull()
    expect(link.className).toContain('h-16')
    expect(link.className).toContain('rounded-none')
    const click = new MouseEvent('click', { bubbles: true, cancelable: true })
    link.dispatchEvent(click)
    expect(click.defaultPrevented).toBe(false)
    expect(clicked).toHaveBeenCalledOnce()
  })
})
it('loading exposes busy state and restores native activation after completion', async () => {
  const onClick = vi.fn()
  const state = reactive({ loading: true, onClick })
  const root = mount(AppButton, state)
  let button = root.querySelector('button')!
  expect(button.disabled).toBe(true)
  expect(button.getAttribute('aria-busy')).toBe('true')
  expect(button.hasAttribute('data-loading')).toBe(true)
  expect(button.getAttribute('data-ui')).toBe('button')
  activation(button)
  expect(onClick).not.toHaveBeenCalled()
  state.loading = false
  await nextTick()
  button = root.querySelector('button')!
  expect(button.disabled).toBe(false)
  expect(button.hasAttribute('data-loading')).toBe(false)
  expect(button.hasAttribute('aria-busy')).toBe(false)
  button.click()
  expect(onClick).toHaveBeenCalledOnce()
})
it('loading asChild links suppress their handler and leave the tab order', () => {
  const onClick = vi.fn()
  const root = mount(AppButton, { loading: true, asChild: true }, () =>
    h('a', { href: '#save', tabindex: 0, onClick }, 'Save'),
  )
  const link = root.querySelector('a')!
  expect(link.getAttribute('aria-busy')).toBe('true')
  expect(link.getAttribute('tabindex')).toBe('-1')
  expect(activation(link).every((event) => event.defaultPrevented)).toBe(true)
  expect(onClick).not.toHaveBeenCalled()
})
