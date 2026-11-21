import { expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { adminFetch } from '@/http'
import { applyLocale } from '@/i18n'
import ReadAccess from './ReadAccess.vue'

vi.mock('@/http', () => ({ adminFetch: vi.fn() }))
vi.mock('@/components/layout/AppHead.vue', () => ({ default: () => null }))

it.each([
  { confirmedAction: false, label: 'Чтение разрешено.' },
  { confirmedAction: true, label: 'Подключение подтверждено.' },
])('guides the operator back to the client after $label', async ({ confirmedAction, label }) => {
  applyLocale('ru')
  const root = document.createElement('div')
  const app = createApp(ReadAccess, {
    grants: [],
    connectPath: '/agent/example/connect',
    title: 'Example',
    description: 'Example connection',
    confirmedAction,
  })
  app.mount(root)
  try {
    const input = root.querySelector('input')!
    input.value = 'ABCDE-FGHIJ'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    vi.mocked(adminFetch).mockResolvedValueOnce(Response.json({ grants: ['connection'] }))
    root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    await vi.waitFor(() =>
      expect(root.querySelector('[role="status"]')?.textContent?.trim()).toBe(
        label + ' Вернитесь в MCP-клиент и повторите запрос.',
      ),
    )

    vi.mocked(adminFetch).mockResolvedValueOnce(Response.json({ grants: [] }))
    root.querySelector('li button')!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await vi.waitFor(() =>
      expect(root.querySelector('[role="status"]')?.textContent?.trim()).toBe('Доступ закрыт.'),
    )
  } finally {
    app.unmount()
    vi.mocked(adminFetch).mockReset()
  }
})
