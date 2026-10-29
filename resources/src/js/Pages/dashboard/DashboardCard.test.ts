import indexSource from './IndexPage.vue?raw'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, reactive, type Component, type Slots } from 'vue'
import { createPinia } from 'pinia'
import DashboardCard from './DashboardCard.vue'
import IndexPage from './IndexPage.vue'
import { dashboardText, type DashboardDisplayCard } from './useDashboard'
import { setLocale } from '@/i18n'
import { useAuthUserStore } from '@/stores/authUser'
import type { AdminAuthUser } from '@/types/models'

const transport = vi.hoisted(() => ({ request: vi.fn() }))
const page = vi.hoisted(() => ({ props: {} as Record<string, unknown> }))
vi.mock('@/http', () => ({
  adminFetch: transport.request,
  sessionCoordinator: { getState: () => 'ready' },
}))
vi.mock('@inertiajs/vue3', async () => {
  const { h } = await import('vue')
  return {
    Link: {
      props: ['href'],
      setup:
        (props: { href: string }, { slots }: { slots: Slots }) =>
        () =>
          h('a', { href: props.href }, slots.default?.()),
    },
    Head: { setup: () => () => null },
    usePage: () => page,
  }
})
const apps: ReturnType<typeof createApp>[] = []
function mount(component: Component, props: Record<string, unknown>) {
  const root = document.createElement('div')
  const app = createApp({ render: () => h(component, props) })
  const pinia = createPinia()
  useAuthUserStore(pinia).setAuthUser({ id: 7 } as unknown as AdminAuthUser)
  app.use(pinia)
  app.mount(root)
  apps.push(app)
  return root
}
const card = (): DashboardDisplayCard => ({
  key: 'host',
  title: { text: 'Title fallback', ru: 'Состояние проекта', en: 'Project status' },
  metrics: [
    { key: 'long', label: { text: 'Label fallback', ru: 'Счётчик', en: 'Counter' }, value: '123' },
  ],
  links: [{ href: '/project', label: { text: 'Open fallback', ru: 'Открыть', en: 'Open' } }],
  status: { text: { text: 'Status fallback', ru: 'Готово', en: 'Ready' }, tone: 'success' },
  state: 'ready',
  updatedAt: '2026-10-29T08:00:00Z',
  hasSuccessfulData: true,
})
beforeEach(() => {
  setLocale('ru')
  localStorage.clear()
  transport.request
    .mockReset()
    .mockImplementation(async () => new Response(JSON.stringify({ owner: '7', cards: [] })))
  page.props = {}
})
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  setLocale('ru')
  vi.restoreAllMocks()
})

it('renders localized host text live in RU and EN, including status and links', async () => {
  const root = mount(DashboardCard, { card: card() })
  expect(root.querySelector('[data-dashboard-card="host"]')).not.toBeNull()
  expect(root.querySelector('[data-dashboard-metric="long"]')?.textContent).toBe('123')
  expect(root.textContent).toContain('Состояние проекта')
  expect(root.textContent).toContain('Счётчик')
  expect(root.textContent).toContain('Готово')
  expect(root.querySelector('a')?.textContent).toContain('Открыть')
  setLocale('en')
  await nextTick()
  expect(root.textContent).toContain('Project status')
  expect(root.textContent).toContain('Counter')
  expect(root.textContent).toContain('Ready')
  expect(root.querySelector('a')?.textContent).toContain('Open')
  expect(root.querySelector('[data-part="indicator"]')).not.toBeNull()
})

it('falls back to original host text without translating it or interpreting HTML', () => {
  const value = card()
  value.title = { text: 'Обновить' }
  value.metrics[0]!.label = { text: '<img src=x onerror="alert(1)">' }
  setLocale('en')
  const root = mount(DashboardCard, { card: value })
  expect(root.querySelector('h2')?.textContent).toContain('Обновить')
  expect(root.querySelector('dt')?.textContent).toContain('<img src=x')
  expect(root.querySelector('img')).toBeNull()
  expect(dashboardText({ text: 'Fallback', ru: 'Русский' }, 'en')).toBe('Fallback')
})

it('shows an explicit first-load failure without a fabricated zero', () => {
  const value = card()
  value.state = 'unavailable'
  value.metrics = []
  value.updatedAt = undefined
  value.hasSuccessfulData = false
  const root = mount(DashboardCard, { card: value })
  expect(root.textContent).toContain('Показатели временно недоступны.')
  expect(root.querySelector('dd')).toBeNull()
  expect(root.querySelector('time')).toBeNull()
  expect(root.textContent).not.toContain('Готово')
})

it('marks retained metrics stale and preserves the actual successful timestamp', () => {
  const value = card()
  value.state = 'unavailable'
  value.stale = true
  const root = mount(DashboardCard, { card: value })
  expect(root.textContent).toContain('Показаны последние полученные данные.')
  expect(root.querySelector('dd')?.textContent).toBe('123')
  expect(root.querySelector('time')?.getAttribute('datetime')).toBe(value.updatedAt)
  expect(root.textContent).toContain('Готово')
})

it('keeps the last status-only result visible as stale and removes it after success without status', async () => {
  const value = card()
  value.metrics = []
  page.props = { dashboard: { owner: '7', cards: [] } }
  transport.request.mockImplementation(
    async () => new Response(JSON.stringify({ owner: '7', cards: [value] })),
  )
  const root = mount(IndexPage, {})
  const flush = async () => {
    for (let index = 0; index < 15; index++) await Promise.resolve()
    await nextTick()
  }
  await flush()
  expect(root.textContent).toContain('Готово')
  const unavailable = { ...value, state: 'unavailable', status: undefined, updatedAt: undefined }
  transport.request.mockImplementation(
    async () => new Response(JSON.stringify({ owner: '7', cards: [unavailable] })),
  )
  root.querySelector('button')?.click()
  await flush()
  expect(root.textContent).toContain('Показаны последние полученные данные.')
  expect(root.textContent).toContain('Готово')
  expect(root.querySelector('time')?.getAttribute('datetime')).toBe(value.updatedAt)
  const staleStatus = [...root.querySelectorAll('[data-ui="status"][data-part="root"]')].find(
    (status) => status.textContent?.includes('Готово'),
  )
  expect(
    staleStatus?.querySelector('[data-part="indicator"]')?.classList.contains('text-text-tertiary'),
  ).toBe(true)
  setLocale('en')
  await nextTick()
  expect(root.textContent).toContain('Showing the last received data')
  expect(root.textContent).toContain('Ready')
  transport.request.mockImplementation(
    async () =>
      new Response(JSON.stringify({ owner: '7', cards: [{ ...value, status: undefined }] })),
  )
  root.querySelector('button')?.click()
  await flush()
  expect(root.textContent).not.toContain('Ready')
  expect(root.textContent).not.toContain('Showing the last received data')
})

it('replaces loading with a clear error instead of a zero', async () => {
  const value = reactive(card())
  value.state = 'loading'
  value.metrics = []
  value.updatedAt = undefined
  value.hasSuccessfulData = false
  const root = mount(DashboardCard, { card: value })
  expect(root.querySelector('[role="status"]')?.textContent).toContain('Загрузка показателей…')
  value.state = 'unavailable'
  await nextTick()
  expect(root.textContent).toContain('Показатели временно недоступны.')
  expect(root.textContent).not.toContain('Загрузка показателей…')
  expect(root.querySelector('dd')).toBeNull()
})

it('uses wrapping constraints for long host text and the shared responsive grid', () => {
  const value = card()
  value.title.text = 'x'.repeat(500)
  value.title.ru = value.title.text
  value.metrics[0]!.value = '9'.repeat(300)
  const root = mount(DashboardCard, { card: value })
  expect(root.querySelector('section')?.classList.contains('min-w-0')).toBe(true)
  for (const selector of ['h2', 'dt', 'dd', 'li']) {
    expect(root.querySelector(selector)?.classList.contains('break-words')).toBe(true)
  }
  expect(indexSource).toContain('grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3')
  expect(indexSource).toContain('flex flex-wrap items-start')
})

it('keeps the existing filtered catalog below cards and wires the manual refresh control', async () => {
  page.props = {
    dashboard: { owner: '7', cards: [card()] },
    adminMenu: {
      sections: [
        { key: 'tools', title: 'Инструменты', items: [{ name: 'Очереди', href: '/queues' }] },
      ],
    },
  }
  const root = mount(IndexPage, {})
  expect(root.querySelector('h2')?.textContent).toContain('Состояние проекта')
  expect(root.textContent?.indexOf('Доступные разделы управления')).toBeGreaterThan(
    root.textContent?.indexOf('Состояние проекта') ?? 0,
  )
  for (let i = 0; i < 15; i++) await Promise.resolve()
  await nextTick()
  expect(root.querySelector('a[href="/queues"]')).not.toBeNull()
  expect(root.querySelector('input[type="checkbox"]')?.hasAttribute('checked')).toBe(false)
  root.querySelector('button')?.click()
  for (let i = 0; i < 15; i++) await Promise.resolve()
  expect(transport.request).toHaveBeenCalledTimes(2)
})
