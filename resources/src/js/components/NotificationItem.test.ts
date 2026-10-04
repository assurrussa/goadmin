import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive } from 'vue'
import type { Notification } from '@/stores/notifications'
import NotificationItem from './NotificationItem.vue'

const cleanup: Array<() => void> = []
afterEach(() => {
  cleanup
    .splice(0)
    .reverse()
    .forEach((dispose) => dispose())
  vi.useRealTimers()
})

function mountItem(overrides: Partial<Notification> = {}) {
  const notification = reactive<Notification>({
    id: 'notification-test',
    type: 'info',
    title: 'Информация',
    message: 'Изменения сохранены.',
    timeout: 0,
    closable: true,
    createdAt: 1,
    ...overrides,
  })
  const onClose = vi.fn()
  const onSubmit = vi.fn((event: Event) => event.preventDefault())
  const root = document.createElement('div')
  document.body.append(root)
  cleanup.push(() => root.remove())
  // Mount inside a form to guard against accidental submit-button behavior.
  const app = createApp({
    render: () => h('form', { onSubmit }, [h(NotificationItem, { notification, onClose })]),
  })
  app.mount(root)
  let mounted = true
  const unmount = () => {
    if (mounted) app.unmount()
    mounted = false
  }
  cleanup.push(unmount)
  const card = root.querySelector<HTMLElement>('.notification')!
  const progress = () => root.querySelector<HTMLElement>('[style]')
  return { unmount, root, card, notification, onClose, onSubmit, progress }
}

describe('notification presentation contract', () => {
  it.each(['success', 'error', 'warning', 'info'] as const)(
    'preserves %s type, message, title and decorative type icon',
    (type) => {
      const { card } = mountItem({ type, title: 'Заголовок', message: 'Текст уведомления' })
      expect(card.classList.contains(`notification--${type}`)).toBe(true)
      expect(card.querySelector('h3')?.textContent).toBe('Заголовок')
      expect(card.querySelector('p')?.textContent).toBe('Текст уведомления')
      expect(card.querySelector('svg')?.classList.contains(`text-${type}`)).toBe(true)
      expect(card.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
    },
  )

  it('keeps long title/message text intact and supplies shrink/wrap utilities', () => {
    const title = 'ОченьДлинныйЗаголовок'.repeat(20)
    const message = `<b>${'https://example.test/'.repeat(40)}</b>`
    const { card } = mountItem({ title, message })
    expect(card.querySelector('h3')?.textContent).toBe(title)
    expect(card.querySelector('p')?.textContent).toBe(message)
    expect(card.querySelector('b')).toBeNull()
    const content = card.querySelector('p')!.parentElement!
    expect(content.classList.contains('min-w-0')).toBe(true)
    expect(content.classList.contains('[overflow-wrap:anywhere]')).toBe(true)
    // jsdom does not lay out text: actual wrapping/overflow is a browser gate.
  })

  it('keeps an absent title absent', () => {
    const { card } = mountItem({ title: undefined })
    expect(card.querySelector('h3')).toBeNull()
    expect(card.querySelector('p')?.textContent).toBe('Изменения сохранены.')
  })

  it('names a native non-submit close button with touch and keyboard-focus utilities', () => {
    const { card, onClose, onSubmit } = mountItem()
    const close = card.querySelector('button')!
    expect(close.type).toBe('button')
    expect(close.getAttribute('aria-label')).toBe('Закрыть уведомление')
    expect(close.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
    for (const utility of ['h-11', 'w-11', 'focus-visible:ring-2', 'focus-visible:ring-ring']) {
      expect(close.classList.contains(utility)).toBe(true)
    }
    close.focus()
    expect(document.activeElement).toBe(close)
    close.click()
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(onSubmit).not.toHaveBeenCalled()
    // Actual 44px size, visible ring, and Enter/Space/tap need a real browser.
  })

  it('does not add a close control to a non-closable notification', () => {
    const { card, onClose } = mountItem({ closable: false })
    expect(card.querySelector('button')).toBeNull()
    expect(onClose).not.toHaveBeenCalled()
  })
})

describe('unchanged notification lifetime', () => {
  it.each([undefined, 0, -1])('does not auto-close with timeout %s', (timeout) => {
    vi.useFakeTimers()
    const { progress, onClose } = mountItem({ timeout })
    expect(progress()).toBeNull()
    vi.advanceTimersByTime(10000)
    expect(onClose).not.toHaveBeenCalled()
  })

  it('preserves progress, hover pause, resume and first expiry timing', async () => {
    vi.useFakeTimers()
    const { card, progress, onClose } = mountItem({ type: 'error', timeout: 1000 })
    await nextTick()
    expect(progress()?.style.width).toBe('100%')
    expect(progress()?.style.backgroundColor).toBe('var(--color-error)')
    vi.advanceTimersByTime(300)
    await nextTick()
    expect(progress()?.style.width).toBe('70%')
    card.dispatchEvent(new MouseEvent('mouseenter'))
    vi.advanceTimersByTime(1000)
    await nextTick()
    expect(progress()?.style.width).toBe('70%')
    expect(progress()?.parentElement?.classList.contains('opacity-60')).toBe(true)
    expect(onClose).not.toHaveBeenCalled()
    card.dispatchEvent(new MouseEvent('mouseleave'))
    vi.advanceTimersByTime(600)
    await nextTick()
    expect(progress()?.style.width).toBe('10%')
    expect(progress()?.parentElement?.classList.contains('opacity-100')).toBe(true)
    expect(onClose).not.toHaveBeenCalled()
    vi.advanceTimersByTime(100)
    await nextTick()
    expect(progress()?.style.width).toBe('0%')
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('preserves the existing remaining-time reset when timeout changes', async () => {
    vi.useFakeTimers()
    const { notification, progress, onClose } = mountItem({ timeout: 1000 })
    vi.advanceTimersByTime(300)
    notification.timeout = 2000
    await nextTick()
    expect(progress()?.style.width).toBe('100%')
    vi.advanceTimersByTime(1900)
    expect(onClose).not.toHaveBeenCalled()
    vi.advanceTimersByTime(100)
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('cleans up its running interval on unmount', () => {
    vi.useFakeTimers()
    const { unmount, onClose } = mountItem({ timeout: 1000 })
    unmount()
    expect(vi.getTimerCount()).toBe(0)
    vi.advanceTimersByTime(2000)
    expect(onClose).not.toHaveBeenCalled()
  })
})
