import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import MFAEnrollmentPage from './MFAEnrollmentPage.vue'

const post = vi.hoisted(() => vi.fn())
vi.mock('axios', () => ({ default: { post } }))
vi.mock('@inertiajs/vue3', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    Link: defineComponent({
      setup:
        (_, { attrs, slots }) =>
        () =>
          h('a', attrs, slots.default?.()),
    }),
  }
})
vi.mock('@/components/layout/AuthLayout.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      setup:
        (_, { slots }) =>
        () =>
          h('div', [slots.title?.(), slots.subtitle?.(), slots.default?.(), slots.footer?.()]),
    }),
  }
})
vi.mock('@/components/form/FormButton.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      inheritAttrs: false,
      setup:
        (_, { attrs, slots }) =>
        () =>
          h('button', attrs, slots.default?.()),
    }),
  }
})
vi.mock('@/components/form/FormInput.vue', async () => {
  const { defineComponent, h } = await import('vue')
  return {
    default: defineComponent({
      props: ['modelValue'],
      emits: ['update:modelValue'],
      setup:
        (props, { attrs, emit }) =>
        () =>
          h('input', {
            ...attrs,
            value: props.modelValue,
            onInput: (event: Event) =>
              emit('update:modelValue', (event.target as HTMLInputElement).value),
          }),
    }),
  }
})

const setup = {
  id: 'setup',
  secret: 'synthetic-secret',
  url: 'otpauth://totp/synthetic',
  expiresAt: '2099-01-01T00:00:00Z',
  recoveryCodes: ['synthetic-recovery'],
}
let app: ReturnType<typeof createApp> | undefined
afterEach(() => {
  app?.unmount()
  app = undefined
  post.mockReset()
  vi.unstubAllGlobals()
})
function mount() {
  const root = document.createElement('div')
  app = createApp(MFAEnrollmentPage, { challengeID: 'pending' })
  app.mount(root)
  return root
}
async function flush() {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}
async function enterCode(root: HTMLElement) {
  const checkbox = root.querySelector<HTMLInputElement>('input[type="checkbox"]')!
  checkbox.checked = true
  checkbox.dispatchEvent(new Event('change', { bubbles: true }))
  const code = root.querySelector<HTMLInputElement>('input[name="code"]')!
  code.value = '123456'
  code.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
}
function mockNavigation() {
  const realWindow = window
  const navigate = vi.fn()
  vi.stubGlobal(
    'window',
    new Proxy(realWindow, {
      get(target, key) {
        if (key === 'location') return { origin: 'https://admin.example.test', assign: navigate }
        const value = Reflect.get(target, key, target)
        return typeof value === 'function' ? value.bind(target) : value
      },
    }),
  )
  return navigate
}

describe('required MFA setup', () => {
  it('clears visible setup immediately on Return to login', async () => {
    post.mockResolvedValue({ data: { enrollment: setup } })
    const root = mount()
    root.querySelector('button')!.click()
    await flush()
    expect(root.textContent).toContain(setup.secret)
    const leave = root.querySelector('a')!
    leave.addEventListener('click', (event) => event.preventDefault())
    leave.click()
    await flush()
    expect(root.textContent).not.toContain(setup.secret)
    expect(root.textContent).not.toContain(setup.recoveryCodes[0])
    expect(root.querySelector<HTMLButtonElement>('button')?.disabled).toBe(true)
  })
  it('discards setup returned after Return to login even before unmount', async () => {
    let resolve!: (value: unknown) => void
    post.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const root = mount()
    root.querySelector('button')!.click()
    const leave = root.querySelector('a')!
    leave.addEventListener('click', (event) => event.preventDefault())
    leave.click()
    resolve({ data: { enrollment: setup } })
    await flush()
    expect(root.textContent).not.toContain(setup.secret)
    expect(root.textContent).not.toContain(setup.recoveryCodes[0])
  })
  it('does not mutate on GET and fences duplicate begin clicks', async () => {
    let resolve!: (value: unknown) => void
    post.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const root = mount()
    expect(post).not.toHaveBeenCalled()
    const button = root.querySelector('button')!
    button.click()
    button.click()
    expect(post).toHaveBeenCalledTimes(1)
    resolve({ data: { enrollment: setup } })
    await flush()
    expect(root.textContent).toContain(setup.secret)
    expect(root.textContent).toContain(setup.recoveryCodes[0])
    expect(root.textContent).toContain('только после подтверждения')
    expect(root.querySelector<HTMLButtonElement>('button[type="submit"]')?.disabled).toBe(true)
  })

  it('discards a late begin response after pagehide', async () => {
    let resolve!: (value: unknown) => void
    post.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const root = mount()
    root.querySelector('button')!.click()
    window.dispatchEvent(new Event('pagehide'))
    resolve({ data: { enrollment: setup } })
    await flush()
    expect(root.textContent).not.toContain(setup.secret)
    expect(root.textContent).not.toContain(setup.recoveryCodes[0])
    root.querySelector('button')!.click()
    expect(post).toHaveBeenCalledTimes(1)
  })

  it('clears displayed secret and inactive codes on unmount', async () => {
    post.mockResolvedValue({ data: { enrollment: setup } })
    const root = mount()
    root.querySelector('button')!.click()
    await flush()
    expect(root.textContent).toContain(setup.secret)
    app!.unmount()
    app = undefined
    expect(root.textContent).not.toContain(setup.secret)
    expect(root.textContent).not.toContain(setup.recoveryCodes[0])
  })

  it.each([
    {
      redirect: '/reports?tab=active#summary',
      target: 'https://admin.example.test/reports?tab=active#summary',
    },
    {
      redirect: '/reports/a%3Fb%23c?next=%2Fqueues#row%20one',
      target: 'https://admin.example.test/reports/a%3Fb%23c?next=%2Fqueues#row%20one',
    },
    {
      redirect: '/%2509/evil.example',
      target: 'https://admin.example.test/%2509/evil.example',
    },
    { redirect: '/.//evil.example', target: 'https://admin.example.test//evil.example' },
    { redirect: '/%2e//evil.example', target: 'https://admin.example.test//evil.example' },
  ])('automatically follows confirmed safe redirect $redirect', async ({ redirect, target }) => {
    const navigate = mockNavigation()
    post.mockResolvedValueOnce({ data: { enrollment: setup } })
    post.mockResolvedValueOnce({ data: { redirect } })
    const root = mount()
    root.querySelector('button')!.click()
    await flush()
    await enterCode(root)
    root
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }))
    await flush()
    expect(post).toHaveBeenLastCalledWith('/auth/mfa/enrollment/confirm', {
      challengeID: 'pending',
      enrollmentID: 'setup',
      code: '123456',
    })
    expect(navigate).toHaveBeenCalledExactlyOnceWith(target)
    expect(root.textContent).not.toContain(setup.secret)
    expect(root.textContent).not.toContain(setup.recoveryCodes[0])
    expect(root.textContent).not.toContain('Коды сохранены, продолжить')
  })

  it.each([
    '//evil.example',
    '/\\evil.example',
    '/\t/evil.example',
    '/\r/evil.example',
    '/\n/evil.example',
    'https://evil.example',
  ])('rejects unsafe confirmation redirect %j', async (redirect) => {
    const navigate = mockNavigation()
    post.mockResolvedValueOnce({ data: { enrollment: setup } })
    post.mockResolvedValueOnce({ data: { redirect } })
    const root = mount()
    root.querySelector('button')!.click()
    await flush()
    await enterCode(root)
    root
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }))
    await flush()
    expect(navigate).not.toHaveBeenCalled()
    expect(root.querySelector('[role="alert"]')?.textContent).toContain(
      'Не удалось подтвердить настройку',
    )
  })

  it('does not follow a late confirmation after leaving the page', async () => {
    const navigate = mockNavigation()
    let resolve!: (value: unknown) => void
    post.mockResolvedValueOnce({ data: { enrollment: setup } })
    post.mockImplementationOnce(
      () =>
        new Promise((done) => {
          resolve = done
        }),
    )
    const root = mount()
    root.querySelector('button')!.click()
    await flush()
    await enterCode(root)
    root
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }))
    window.dispatchEvent(new Event('pagehide'))
    resolve({ data: { redirect: '/reports' } })
    await flush()
    expect(navigate).not.toHaveBeenCalled()
    expect(root.textContent).not.toContain(setup.secret)
    expect(root.textContent).not.toContain(setup.recoveryCodes[0])
  })
})
