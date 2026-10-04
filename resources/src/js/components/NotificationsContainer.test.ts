import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createPinia } from 'pinia'
import { useNotificationsStore } from '@/stores/notifications'
import NotificationsContainer from './NotificationsContainer.vue'

const cleanup: Array<() => void> = []
afterEach(() => {
  cleanup
    .splice(0)
    .reverse()
    .forEach((dispose) => dispose())
  vi.useRealTimers()
})

function mountContainer() {
  const pinia = createPinia()
  const store = useNotificationsStore(pinia)
  const root = document.createElement('div')
  document.body.append(root)
  cleanup.push(() => root.remove())
  const app = createApp(NotificationsContainer)
  app.use(pinia)
  app.mount(root)
  cleanup.push(() => app.unmount())
  return { root, store }
}

it('keeps the desktop anchor/max width and subtracts both mobile margins', async () => {
  const { root, store } = mountContainer()
  expect(document.querySelector('.notification')).toBeNull()
  store.error('Ошибка входа', { timeout: 0 })
  await nextTick()
  const card = document.querySelector('.notification')!
  expect(root.contains(card)).toBe(false)
  const toastContainer = card.parentElement!.parentElement!
  for (const utility of ['fixed', 'top-4', 'right-4', 'max-w-sm', 'w-[calc(100%-2rem)]']) {
    expect(toastContainer.classList.contains(utility)).toBe(true)
  }
  expect(toastContainer.classList.contains('w-full')).toBe(false)
  // Class contract only: compiled CSS and 320/390/1440px bounds require browser QA.
})

it('removes only the selected notification and retains the other content and defaults', async () => {
  const { store } = mountContainer()
  const first = store.error('Первая ошибка', { timeout: 0 })
  const second = store.success('Изменение сохранено', { timeout: 0 })
  await nextTick()
  expect(store.notifications.map(({ id }) => id)).toEqual([first, second])
  const buttons = document.querySelectorAll<HTMLButtonElement>('.notification button')
  expect(buttons).toHaveLength(2)
  buttons[0].click()
  await nextTick()
  expect(store.notifications).toHaveLength(1)
  expect(store.notifications[0]).toMatchObject({
    id: second,
    title: 'Успешно',
    message: 'Изменение сохранено',
    type: 'success',
    closable: true,
    timeout: 0,
  })
})

it('retains default timeout and automatically removes even a non-closable notification', async () => {
  vi.useFakeTimers()
  const { store } = mountContainer()
  store.info('Временное уведомление', { closable: false })
  await nextTick()
  expect(store.notifications[0].timeout).toBe(5000)
  expect(document.querySelector('.notification button')).toBeNull()
  vi.advanceTimersByTime(4900)
  expect(store.notifications).toHaveLength(1)
  vi.advanceTimersByTime(100)
  await nextTick()
  expect(store.notifications).toHaveLength(0)
})
