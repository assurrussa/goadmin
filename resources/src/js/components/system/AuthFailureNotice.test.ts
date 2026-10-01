// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { createApp, Fragment, h, nextTick, ref } from 'vue'
import { router } from '@inertiajs/vue3'
import AuthFailureNotice from './AuthFailureNotice.vue'
import { createAuthFailureHandler } from '@/auth/browserAuthFailures'

const cleanup: Array<() => void> = []
afterEach(() => {
  cleanup
    .splice(0)
    .reverse()
    .forEach((dispose) => dispose())
})

function authResponse(status: number, code: string) {
  return {
    status,
    data: { status, code, message: 'server message' },
    headers: { 'x-goadmin-auth-error': '1', 'content-type': 'application/json' },
    config: { url: '/protected' },
  }
}

it('handles Inertia invalid without replacing the form or resubmitting it', async () => {
  const message = ref('')
  const submit = vi.fn((event: Event) => event.preventDefault())
  const navigate = vi.fn()
  const clearAuth = vi.fn()
  const disconnect = vi.fn()
  const element = document.createElement('div')
  document.body.append(element)
  cleanup.push(() => element.remove())
  const app = createApp({
    render: () =>
      h(Fragment, null, [
        h('form', { onSubmit: submit }, [h('input', { name: 'draft' })]),
        h(AuthFailureNotice, {
          message: message.value,
          onDismiss: () => {
            message.value = ''
          },
        }),
      ]),
  })
  app.mount(element)
  cleanup.push(() => app.unmount())
  cleanup.push(
    router.on(
      'invalid',
      createAuthFailureHandler({
        origin: window.location.origin,
        clearAuth,
        disconnect,
        navigateToLogin: navigate,
        showNotice: (notice) => {
          message.value = notice
        },
      }),
    ),
  )
  const input = element.querySelector('input')!
  input.value = 'unsaved draft'
  const event = new CustomEvent('inertia:invalid', {
    cancelable: true,
    detail: { response: authResponse(503, 'authentication_retry_required') },
  })
  expect(document.dispatchEvent(event)).toBe(false)
  await nextTick()
  expect(element.querySelector('input')).toBe(input)
  expect(input.value).toBe('unsaved draft')
  expect(element.querySelector('[role="alert"]')?.textContent).toContain('вручную')
  expect(submit).not.toHaveBeenCalled()
  expect(navigate).not.toHaveBeenCalled()
  expect(clearAuth).not.toHaveBeenCalled()
  expect(disconnect).not.toHaveBeenCalled()
  element.querySelector<HTMLButtonElement>('aside button')!.click()
  await nextTick()
  expect(element.querySelector('[role="alert"]')).toBeNull()
  expect(input.value).toBe('unsaved draft')
})

it('reauthentication clears state and disconnects before separate navigation', () => {
  const calls: string[] = []
  cleanup.push(
    router.on(
      'invalid',
      createAuthFailureHandler({
        origin: window.location.origin,
        clearAuth: () => {
          calls.push('clear')
        },
        disconnect: () => {
          calls.push('disconnect')
        },
        navigateToLogin: () => {
          calls.push('GET /auth/login')
        },
        showNotice: vi.fn(),
      }),
    ),
  )
  const event = new CustomEvent('inertia:invalid', {
    cancelable: true,
    detail: { response: authResponse(401, 'reauthentication_required') },
  })
  expect(document.dispatchEvent(event)).toBe(false)
  expect(calls).toEqual(['clear', 'disconnect', 'GET /auth/login'])
})

it('does not suppress an unrelated invalid response', () => {
  cleanup.push(
    router.on(
      'invalid',
      createAuthFailureHandler({
        origin: window.location.origin,
        clearAuth: vi.fn(),
        disconnect: vi.fn(),
        navigateToLogin: vi.fn(),
        showNotice: vi.fn(),
      }),
    ),
  )
  const event = new CustomEvent('inertia:invalid', {
    cancelable: true,
    detail: { response: { status: 500, data: '<p>unexpected server error</p>' } },
  })
  expect(document.dispatchEvent(event)).toBe(true)
})
