import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import { applyLocale } from '@/i18n'
import LocalizedDatePicker from './LocalizedDatePicker.vue'

let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  document.body.innerHTML = ''
  applyLocale('ru')
  vi.unstubAllGlobals()
})
function mount(initial: string | null, extra: Record<string, unknown> = {}) {
  const state = reactive({ modelValue: initial, ...extra })
  const update = vi.fn((value: string | null) => {
    state.modelValue = value
  })
  const valid = vi.fn()
  const root = document.createElement('form')
  document.body.append(root)
  app = createApp({
    render: () =>
      h(LocalizedDatePicker, { ...state, 'onUpdate:modelValue': update, 'onUpdate:valid': valid }),
  })
  app.mount(root)
  return { root, state, update, valid }
}
async function edit(root: HTMLElement, part: string, value: string) {
  const input = root.querySelector<HTMLInputElement>(`[data-date-part=${part}]`)!
  input.value = value
  input.dispatchEvent(new Event('input'))
  await nextTick()
}
it('changes RU/EN separators live while preserving Gregorian date and 24-hour local time', async () => {
  const { root, state, update } = mount('2024-02-29T00:05', { withTime: true, label: 'Expiry' })
  expect(
    [...root.querySelectorAll('.date-control-separator')].map((node) => node.textContent),
  ).toEqual(['.', '.', ' ', ':'])
  applyLocale('en')
  await nextTick()
  expect(
    [...root.querySelectorAll('.date-control-separator')].map((node) => node.textContent),
  ).toEqual(['/', '/', ' ', ':'])
  expect(root.querySelector<HTMLInputElement>('[data-date-part=hour]')!.value).toBe('00')
  expect(state.modelValue).toBe('2024-02-29T00:05')
  expect(update).not.toHaveBeenCalled()
  await edit(root, 'minute', '45')
  expect(state.modelValue).toBe('2024-02-29T00:45')
})
it('blocks partial and impossible leap dates and clears the previous wire value immediately', async () => {
  const { root, state, valid } = mount('2024-02-29')
  await edit(root, 'year', '2025')
  expect(state.modelValue).toBe('')
  expect(valid).toHaveBeenLastCalledWith(false)
  expect(root.checkValidity()).toBe(false)
  expect(root.querySelector('[role=alert]')).not.toBeNull()
  await edit(root, 'year', '2024')
  expect(state.modelValue).toBe('2024-02-29')
  expect(root.checkValidity()).toBe(true)
  await edit(root, 'month', '')
  applyLocale('en')
  await nextTick()
  expect(state.modelValue).toBe('')
  expect(root.querySelector<HTMLInputElement>('[data-date-part=day]')!.value).toBe('29')
  expect(root.querySelector<HTMLInputElement>('[data-date-part=month]')!.value).toBe('')
  expect(root.checkValidity()).toBe(false)
})
it('supports optional clear, untouched null, required state, readonly and disabled fencing', async () => {
  const { root, state, update } = mount(null)
  await nextTick()
  expect(state.modelValue).toBeNull()
  expect(update).not.toHaveBeenCalled()
  expect(root.checkValidity()).toBe(true)
  state.modelValue = '2026-11-06'
  await nextTick()
  root.querySelector<HTMLButtonElement>('button[aria-label="Очистить дату"]')!.click()
  await nextTick()
  expect(state.modelValue).toBe('')
  expect(root.checkValidity()).toBe(true)
  Object.assign(state, { modelValue: '2026-11-06', readonly: true })
  await nextTick()
  await edit(root, 'day', '01')
  expect(state.modelValue).toBe('2026-11-06')
  Object.assign(state, { readonly: false, disabled: true })
  await nextTick()
  expect([...root.querySelectorAll('button')].every((button) => button.disabled)).toBe(true)
  Object.assign(state, { disabled: false, required: true, modelValue: '' })
  await nextTick()
  expect(root.checkValidity()).toBe(false)
  expect(root.querySelector('button[aria-label="Очистить дату"]')).toBeNull()
})
it('supports keyboard segment navigation and numeric adjustment without timezone shifts', async () => {
  const { root, state } = mount('2026-11-06')
  const day = root.querySelector<HTMLInputElement>('[data-date-part=day]')!
  day.focus()
  day.setSelectionRange(2, 2)
  day.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true, cancelable: true }),
  )
  expect(document.activeElement?.getAttribute('data-date-part')).toBe('month')
  const month = document.activeElement!
  month.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'ArrowUp', bubbles: true, cancelable: true }),
  )
  await nextTick()
  expect(state.modelValue).toBe('2026-12-06')
})
it('uses a Monday-first localized calendar and preserves wall-clock time on day selection', async () => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  const { root, state } = mount('2026-11-06T23:45', { withTime: true, label: 'Expiry' })
  root.querySelector<HTMLButtonElement>('button[aria-label="Открыть календарь"]')!.click()
  await nextTick()
  await nextTick()
  expect(document.querySelector('.date-control-weekday')!.textContent).toMatch(/пн/i)
  const day = [...document.querySelectorAll<HTMLButtonElement>('.date-control-day')].find(
    (button) => button.getAttribute('aria-label')?.includes('7 ноября'),
  )!
  expect(day).toBeDefined()
  day.click()
  await nextTick()
  expect(state.modelValue).toBe('2026-11-07T23:45')
})

it.each([
  ['0001-01-01', 'Предыдущий месяц'],
  ['9999-12-31', 'Следующий месяц'],
])('fences calendar days outside the ISO wire range at %s', async (initial, navigation) => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  const { root, state } = mount(initial)
  root.querySelector<HTMLButtonElement>('button[aria-label="Открыть календарь"]')!.click()
  await nextTick()
  await nextTick()
  const control = document.querySelector<HTMLButtonElement>(
    `.date-control-popover button[aria-label="${navigation}"]`,
  )!
  const heading = document.querySelector('.date-control-header')!.textContent
  control.click()
  await nextTick()
  expect(document.querySelector('.date-control-header')!.textContent).toBe(heading)
  expect(state.modelValue).toBe(initial)
})
