import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive } from 'vue'
import ForgotPasswordPage from './ForgotPasswordPage.vue'

const { post } = vi.hoisted(() => ({ post: vi.fn() }))
let form: { email: string; errors: Record<string, string>; processing: boolean }
vi.mock('@inertiajs/vue3', () => ({
  Link: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('a', attrs, slots.default?.()),
  }),
  usePage: () => ({ props: {} }),
  useForm: (data: { email: string }) => {
    form = reactive({ ...data, errors: {}, post, processing: false })
    return form
  },
  router: { on: () => () => {} },
}))
vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('div', [slots.title?.(), slots.subtitle?.(), slots.default?.(), slots.footer?.()]),
  }),
}))

let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  post.mockReset()
})

function render() {
  const root = document.createElement('div')
  app = createApp(ForgotPasswordPage)
  app.mount(root)
  return root
}

describe('password recovery request copy', () => {
  it('asks for a request without promising email delivery', () => {
    const root = render()
    expect(root.textContent).toContain('Укажите email, чтобы запросить восстановление пароля')
    expect(root.textContent).toContain('Принятие запроса не подтверждает отправку письма')
    expect(root.querySelector('button[type="submit"]')?.textContent).toContain(
      'Запросить восстановление',
    )
    expect(root.textContent).not.toMatch(/мы отправим|письмо отправлено|ссылка отправлена/i)
    expect(post).not.toHaveBeenCalled()
  })

  it('submits the existing endpoint and keeps pending requests distinct from sent mail', async () => {
    const root = render()
    form.email = 'fixture@example.test'
    root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    expect(post).toHaveBeenCalledExactlyOnceWith('/auth/forgot-password')
    form.processing = true
    await nextTick()
    expect(root.querySelector('button[type="submit"]')?.textContent).toContain(
      'Обрабатываем запрос...',
    )
    expect(root.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)
    expect(root.textContent).not.toMatch(/запрос принят|письмо отправлено|ссылка отправлена/i)
    root.querySelector('form')!.dispatchEvent(new Event('submit', { cancelable: true }))
    expect(post).toHaveBeenCalledTimes(1)
  })
})
