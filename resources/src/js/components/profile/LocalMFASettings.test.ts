import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import LocalMFASettings from './LocalMFASettings.vue'
const post = vi.hoisted(() => vi.fn())
vi.mock('axios', () => ({
  default: {
    post,
    isAxiosError: (error: unknown) => Boolean((error as { response?: unknown })?.response),
  },
}))
vi.mock('qrcode', () => ({
  default: { toDataURL: vi.fn(async () => 'data:image/png;base64,synthetic') },
}))
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  post.mockReset()
})
function mount(state: 'disabled' | 'enabled' | 'unavailable' | 'unknown') {
  const root = document.createElement('div')
  app = createApp(LocalMFASettings, { state })
  app.mount(root)
  return root
}
async function flush() {
  for (let i = 0; i < 8; i++) await nextTick()
}
function button(root: HTMLElement, text: string) {
  return [...root.querySelectorAll('button')].find((item) => item.textContent?.includes(text))!
}
it.each(['unavailable', 'unknown'] as const)(
  '%s never offers activation or implies disabled',
  (state) => {
    const root = mount(state)
    expect(root.querySelector('input')).toBeNull()
    expect(root.textContent).not.toContain('Защита не включена')
    expect(post).not.toHaveBeenCalled()
  },
)
it('shows local QR and secret only during unconfirmed enrollment, cancellation clears both', async () => {
  const root = mount('disabled')
  expect(root.querySelector('input[name="mfaCode"]')).toBeNull()
  post.mockResolvedValueOnce({
    data: {
      signedOut: false,
      enrollment: { id: 'setup', secret: 'synthetic-secret', url: 'otpauth://totp/test' },
    },
  })
  button(root, 'Включить защиту').click()
  await flush()
  expect(root.querySelector('img')?.getAttribute('src')).toBe('data:image/png;base64,synthetic')
  expect(root.querySelector('input[name="mfaCode"]')).not.toBeNull()
  expect(root.textContent).toContain('synthetic-secret')
  expect(root.textContent).toContain('До подтверждения защита не меняется')
  post.mockResolvedValueOnce({ data: { signedOut: false } })
  button(root, 'Отмена').click()
  await flush()
  expect(root.querySelector('img')).toBeNull()
  expect(root.querySelector('input[name="mfaCode"]')).toBeNull()
  expect(root.textContent).not.toContain('synthetic-secret')
})
it('uncertain mutation clears secrets and prevents automatic or manual replay', async () => {
  const root = mount('enabled')
  post.mockRejectedValueOnce(new Error('lost response'))
  button(root, 'Заменить приложение').click()
  await flush()
  expect(post).toHaveBeenCalledTimes(1)
  expect(root.querySelector('input')).toBeNull()
  expect(root.textContent).toContain('Состояние защиты неизвестно')
  expect(button(root, 'Заменить приложение')).toBeUndefined()
})
it('late enrollment response after pagehide cannot restore secret or QR', async () => {
  let resolve!: (value: unknown) => void
  post.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done
    }),
  )
  const root = mount('disabled')
  button(root, 'Включить защиту').click()
  window.dispatchEvent(new Event('pagehide'))
  resolve({
    data: {
      signedOut: false,
      enrollment: { id: 'setup', secret: 'late-secret', url: 'otpauth://totp/test' },
    },
  })
  await flush()
  expect(root.querySelector('img')).toBeNull()
  expect(root.textContent).not.toContain('late-secret')
})
