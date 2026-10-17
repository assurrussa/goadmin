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

it.each([
 [400, 'proof_invalid', 'Проверьте текущий пароль', true],
 [400, 'verification_rejected', 'Проверка отклонена', true],
 [429, 'rate_limited', 'Слишком много попыток', true],
 [401, 'signin_required', 'Сеанс завершён', false],
 [503, 'mfa_unavailable', 'не означает, что защита выключена', false],
 [503, 'service_unavailable', 'Сервис не подтвердил результат', false],
 [503, 'outcome_unknown', 'Сервис не подтвердил результат', false],
] as const)('classifies HTTP %s %s without treating operational failure as bad proof', async (status, code, text, canRetry) => {
 const root = mount('enabled')
 post.mockRejectedValueOnce({response:{status,data:{code}}})
 button(root,'Заменить приложение').click()
 await flush()
 expect(root.textContent).toContain(text)
 expect(Boolean(root.querySelector('input[name="mfaPassword"]'))).toBe(canRetry)
 expect(root.textContent).not.toContain('Защита не включена')
 if(code === 'signin_required') expect(root.querySelector('a[href="/auth/login"]')).not.toBeNull()
 expect(post).toHaveBeenCalledTimes(1)
})
