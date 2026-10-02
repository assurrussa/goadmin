import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import Demo from '../../../audit/datagrid/Demo.vue'
let app: App | undefined
const originalFetch = window.fetch
afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.innerHTML = ''
  window.fetch = originalFetch
  delete window.AdminDataGrid
  vi.useRealTimers()
})
it('runs the unchanged grid inside the instrumented demo and validates host-form simulation', async () => {
  vi.useFakeTimers()
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp(Demo)
  app.config.warnHandler = () => {}
  app.mount(root)
  await nextTick()
  expect(root.querySelectorAll('tbody tr')).toHaveLength(100)
  expect(root.textContent).toContain('host.permission.resolve: 100')
  const button = (label: string) =>
    [...root.querySelectorAll<HTMLButtonElement>('button')].find(
      (el) => el.textContent?.trim() === label,
    )!
  const click = async (label: string) => {
    // Vue guards events created at the same timestamp as nested listeners.
    // Advance the fake wall clock so the capture logger does not suppress them.
    vi.setSystemTime(Date.now() + 1)
    button(label).click()
    for (let i = 0; i < 8; i++) await nextTick()
  }
  await click('Создать запись')
  await nextTick()
  expect(root.querySelector('.lab-form')).not.toBeNull()
  await click('Save fixture')
  await nextTick()
  expect(root.querySelector('[role="alert"]')?.textContent).toBe('Название обязательно')
  const name = root.querySelector<HTMLInputElement>('[name="fixture-name"]')!
  name.value = 'Created from demo'
  name.dispatchEvent(new Event('input', { bubbles: true }))
  await click('Save fixture')
  await vi.advanceTimersByTimeAsync(60)
  await nextTick()
  expect(root.querySelector('.lab-form')).toBeNull()
  expect(root.textContent).toContain('host.form.saved: 1')
  expect(root.textContent).toContain('callback.action-create: 1')
  await click('Next 403')
  await vi.advanceTimersByTimeAsync(60)
  await nextTick()
  expect(root.querySelector('[role="alert"]')?.textContent).toContain('нет доступа')
  await click('Повторить')
  await vi.advanceTimersByTimeAsync(60)
  await nextTick()
  expect(root.querySelector('[role="alert"]')).toBeNull()
  await click('Unmount grid')
  await nextTick()
  expect(root.querySelector('table')).toBeNull()
  await click('Mount grid')
  await nextTick()
  expect(root.querySelectorAll('tbody tr')).toHaveLength(100)
}, 15000)
