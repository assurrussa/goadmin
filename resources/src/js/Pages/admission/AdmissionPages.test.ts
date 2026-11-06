const confirmation = vi.hoisted(() => vi.fn<(...args: unknown[]) => Promise<boolean>>())
vi.mock('@/composables/useConfirmation', () => ({
  useConfirmation: () => async (options: unknown, onConfirm: () => void | Promise<void>) => {
    const accepted = await confirmation(options)
    if (accepted) await onConfirm()
    return accepted
  },
}))
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick } from 'vue'
import WaitingPage from '../auth/AdmissionWaitingPage.vue'
import IndexPage from './IndexPage.vue'
import { setLocale } from '@/i18n'

const post = vi.hoisted(() => vi.fn())
vi.mock('@inertiajs/vue3', () => ({
  router: { post, on: () => () => {} },
}))
vi.mock('@/components/layout/AuthLayout.vue', () => ({
  default: defineComponent({
    setup:
      (_, { slots }) =>
      () =>
        h('div', slots.default?.()),
  }),
}))
vi.mock('@/components/layout/AppHead.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/layout/PageActionBar.vue', () => ({
  default: defineComponent({ render: () => null }),
}))
vi.mock('@/components/ui/AppButton.vue', () => ({
  default: defineComponent({
    setup:
      (_, { attrs, slots }) =>
      () =>
        h('button', attrs, slots.default?.()),
  }),
}))
vi.mock('@/components/form/FormInput.vue', () => ({
  default: defineComponent({
    props: ['modelValue', 'name', 'label'],
    emits: ['update:modelValue'],
    setup:
      (props, { emit }) =>
      () =>
        h('label', [
          props.label,
          h('input', {
            name: props.name,
            value: props.modelValue,
            onInput: (event: Event) =>
              emit('update:modelValue', (event.target as HTMLInputElement).value),
          }),
        ]),
  }),
}))
const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
  document.body.innerHTML = ''
  post.mockReset()
  confirmation.mockReset()
  setLocale('ru')
})
describe('SSO waiting isolation', () => {
  it.each(['pending', 'approved', 'active', 'rejected'])(
    'renders %s without admin controls',
    (status) => {
      const root = document.createElement('div')
      const app = createApp(WaitingPage, { admissionStatus: status, signInPath: '/sso/login' })
      apps.push(app)
      app.mount(root)
      expect(root.querySelector('[role="status"]')).not.toBeNull()
      expect(root.querySelector('a')?.getAttribute('href')).toBe('/sso/login')
      expect(root.querySelector('form, input, select')).toBeNull()
      expect(post).not.toHaveBeenCalled()
    },
  )
  it.each(['https://foreign.example', '//foreign.example', '/\\foreign.example'])(
    'rejects unsafe retry %s',
    (path) => {
      const root = document.createElement('div')
      const app = createApp(WaitingPage, { admissionStatus: 'pending', signInPath: path })
      apps.push(app)
      app.mount(root)
      expect(root.querySelector('a')?.getAttribute('href')).toBe('/auth/login')
    },
  )
  it('updates RU/EN without issuing a request', async () => {
    const root = document.createElement('div')
    const app = createApp(WaitingPage, { admissionStatus: 'pending', signInPath: '/sso/login' })
    apps.push(app)
    app.mount(root)
    expect(root.textContent).toContain('Ожидайте')
    setLocale('en')
    await nextTick()
    expect(root.textContent).toContain('Wait for an administrator')
    expect(post).not.toHaveBeenCalled()
  })
})
describe('SSO moderation UI', () => {
  async function moderator() {
    const root = document.createElement('div')
    document.body.append(root)
    const app = createApp(IndexPage, {
      records: [{ ID: 'request', ExternalSubject: 'user', Status: 'pending', Version: 4 }],
      invitations: [],
      inviteRoles: [{ ID: 1, PublicID: 'admin', Name: 'Admin' }],
      approveRoles: [{ ID: 1, PublicID: 'admin', Name: 'Admin' }],
      canInvite: true,
      canReject: true,
    })
    apps.push(app)
    app.mount(root)
    const subject = root.querySelector<HTMLInputElement>('input[name=subject]')!
    subject.value = 'user'
    subject.dispatchEvent(new Event('input'))
    for (const select of root.querySelectorAll('select')) {
      select.options[0]!.selected = true
      select.dispatchEvent(new Event('change'))
    }
    for (const [part, value] of Object.entries({
      day: '06',
      month: '11',
      year: '2026',
      hour: '23',
      minute: '45',
    })) {
      const input = root.querySelector<HTMLInputElement>(`[data-date-part=${part}]`)!
      input.value = value
      input.dispatchEvent(new Event('input'))
      await nextTick()
    }
    return root
  }
  it('converts local minute expiry to the existing UTC payload for invite and approval', async () => {
    const root = await moderator()
    const invite = [...root.querySelectorAll('button')].find((button) =>
      button.textContent!.includes('Создать приглашение'),
    )!
    invite.click()
    expect(post).toHaveBeenLastCalledWith(
      '/admissions/invitations',
      {
        subject: 'user',
        roleIds: [1],
        expiresAt: new Date('2026-11-06T23:45').toISOString(),
      },
      expect.anything(),
    )
    post.mock.calls[0]![2].onFinish()
    await nextTick()
    const approve = [...root.querySelectorAll('button')].find((button) =>
      button.textContent!.includes('Одобрить доступ'),
    )!
    approve.click()
    expect(post).toHaveBeenLastCalledWith(
      '/admissions/approve',
      {
        subject: 'user',
        version: 4,
        roleIds: [1],
        expiresAt: new Date('2026-11-06T23:45').toISOString(),
      },
      expect.anything(),
    )
  })
  it('blocks both mutations after a previously complete expiry becomes partial', async () => {
    const root = await moderator()
    const minute = root.querySelector<HTMLInputElement>('[data-date-part=minute]')!
    minute.value = ''
    minute.dispatchEvent(new Event('input'))
    await nextTick()
    for (const button of root.querySelectorAll('button')) {
      if (/Создать приглашение|Одобрить доступ/.test(button.textContent ?? '')) {
        expect(button.disabled).toBe(true)
        button.dispatchEvent(new Event('click'))
      }
    }
    expect(post).not.toHaveBeenCalled()
  })
  it.each([true, false])('waits for rejection confirmation accepted=%s', async (accepted) => {
    let resolve!: (accepted: boolean) => void
    confirmation.mockReturnValue(
      new Promise<boolean>((done) => {
        resolve = done
      }),
    )
    const root = await moderator()
    const reject = [...root.querySelectorAll('button')].find((button) =>
      button.textContent!.includes('Отклонить'),
    )!
    reject.click()
    expect(post).not.toHaveBeenCalled()
    expect(confirmation).toHaveBeenCalledWith({
      message: 'Admission confirm reject',
      confirmLabel: 'Отклонить',
      variant: 'danger',
    })
    resolve(accepted)
    await nextTick()
    if (accepted)
      expect(post).toHaveBeenCalledWith(
        '/admissions/reject',
        { subject: 'user', version: 4 },
        expect.anything(),
      )
    else expect(post).not.toHaveBeenCalled()
  })
  it.each([true, false])(
    'waits for invitation revocation confirmation accepted=%s',
    async (accepted) => {
      let resolve!: (accepted: boolean) => void
      confirmation.mockReturnValue(
        new Promise<boolean>((done) => {
          resolve = done
        }),
      )
      const root = document.createElement('div')
      const invitation = {
        ID: 'invite-1',
        ExternalSubject: 'user',
        ExpiresAt: '2030-01-01',
        Revoked: false,
        AcceptedSubject: '',
      }
      const app = createApp(IndexPage, {
        records: [],
        invitations: [invitation],
        inviteRoles: [],
        approveRoles: [],
        canInvite: true,
        canReject: false,
      })
      apps.push(app)
      app.mount(root)
      const revoke = [...root.querySelectorAll('button')].find((button) =>
        button.textContent!.includes('Отозвать'),
      )!
      revoke.click()
      expect(post).not.toHaveBeenCalled()
      invitation.ID = 'changed'
      expect(confirmation).toHaveBeenCalledWith({
        message: 'Admission confirm revoke',
        confirmLabel: 'Отозвать',
        variant: 'danger',
      })
      resolve(accepted)
      await nextTick()
      if (accepted)
        expect(post).toHaveBeenCalledWith(
          '/admissions/revoke',
          { id: 'invite-1' },
          expect.anything(),
        )
      else expect(post).not.toHaveBeenCalled()
    },
  )
  it('has no enabled mutation for a read-only view', () => {
    const root = document.createElement('div')
    const app = createApp(IndexPage, {
      records: [],
      invitations: [],
      inviteRoles: [],
      approveRoles: [],
      canInvite: false,
      canReject: false,
    })
    apps.push(app)
    app.mount(root)
    for (const button of root.querySelectorAll('button')) expect(button.disabled).toBe(true)
    expect(post).not.toHaveBeenCalled()
  })
})
