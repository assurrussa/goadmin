import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import FormCheckbox from './FormCheckbox.vue'

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => apps.splice(0).forEach((app) => app.unmount()))
function mount(modelValue: boolean | null, disabled = false) {
  const state = reactive({ modelValue, disabled })
  const updated = vi.fn((value: boolean) => {
    state.modelValue = value
  })
  const root = document.createElement('div')
  const app = createApp({
    render: () =>
      h(FormCheckbox, {
        ...state,
        id: 'test-checkbox',
        label: 'Enabled',
        'onUpdate:modelValue': updated,
      }),
  })
  apps.push(app)
  app.mount(root)
  const checkbox = root.querySelector<HTMLButtonElement>('[role="checkbox"]')!
  return { state, updated, checkbox }
}

it.each([true, false, null])(
  'reflects controlled initial modelValue=%s through actual Reka state',
  (value) => {
    const { checkbox } = mount(value)
    expect(checkbox.getAttribute('aria-checked')).toBe(String(value ?? false))
    expect(checkbox.getAttribute('data-state')).toBe(value === true ? 'checked' : 'unchecked')
  },
)

it('tracks external model changes, including null as unchecked', async () => {
  const { state, checkbox, updated } = mount(true)
  for (const value of [false, true, null, true]) {
    state.modelValue = value
    await nextTick()
    expect(checkbox.getAttribute('aria-checked')).toBe(String(value ?? false))
  }
  expect(updated).not.toHaveBeenCalled()
})

it('emits boolean false/true on clicks and updates its controlled checked state', async () => {
  const { checkbox, updated } = mount(true)
  checkbox.click()
  await nextTick()
  expect(updated).toHaveBeenNthCalledWith(1, false)
  expect(checkbox.getAttribute('aria-checked')).toBe('false')
  checkbox.click()
  await nextTick()
  expect(updated).toHaveBeenNthCalledWith(2, true)
  expect(checkbox.getAttribute('aria-checked')).toBe('true')
  expect(updated).toHaveBeenCalledTimes(2)
})

it('keeps disabled checked state and does not emit through native or dispatched clicks', async () => {
  const { checkbox, updated } = mount(true, true)
  expect(checkbox.disabled).toBe(true)
  expect(checkbox.getAttribute('aria-checked')).toBe('true')
  checkbox.click()
  checkbox.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
  await nextTick()
  expect(updated).not.toHaveBeenCalled()
  expect(checkbox.getAttribute('aria-checked')).toBe('true')
})
