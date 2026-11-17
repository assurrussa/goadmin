import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { router } from '@inertiajs/vue3'
import { createApp, h, nextTick, reactive } from 'vue'
import { createPinia } from 'pinia'
import { applyLocale } from '@/i18n'
import { installPresentation } from '@/presentation/context'
import AdminForm from '@/components/form/AdminForm.vue'
import TopbarShell from '../../../../examples/theme-topbar/src/AdminShell.vue'
import AdminLayout from './AdminLayout.vue'

type Item = { name: string; href?: string; children?: Item[] }
type Section = { key: string; title?: string; placement?: string; items: Item[] }

vi.mock('@inertiajs/vue3', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@inertiajs/vue3')>()),
  usePage: () => page,
}))

const page = reactive({
  props: { adminMenu: { sections: [] as Section[] } },
  url: '/',
})

let app: ReturnType<typeof createApp> | undefined
beforeEach(() => {
  applyLocale('ru')
  page.props.adminMenu.sections = [
    {
      key: 'access',
      title: 'Доступ',
      items: [
        {
          name: 'Permissions group',
          children: [{ name: 'Администраторы', href: '/admins' }],
        },
        {
          name: 'Роли',
          href: '/roles',
          children: [{ name: 'Разрешения', href: '/roles/permissions' }],
        },
        { name: 'Выйти', href: '/auth/logout?source=menu' },
      ],
    },
    {
      key: 'account',
      placement: 'account',
      items: [
        { name: 'Account connection', href: '/auth/profile/settings#connection' },
        { name: 'Unlisted member', children: [{ name: 'Nested account', href: '/child' }] },
        { name: 'Outside link', href: '//external.example' },
      ],
    },
  ]
  vi.stubGlobal('matchMedia', (media: string) => ({
    media,
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
})

afterEach(() => {
  app?.unmount()
  app = undefined
  document.body.innerHTML = ''
  delete document.documentElement.dataset.adminTheme
  applyLocale('ru')
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function mount(shell?: typeof TopbarShell, dirtyForm = false) {
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp({
    render: () =>
      h(AdminLayout, null, {
        default: () => [
          h('button', { id: 'opener' }, 'Open from here'),
          h('textarea', { id: 'editor' }),
          h('div', { contenteditable: true }, [h('span', { id: 'editable-child' })]),
          dirtyForm
            ? h(AdminForm, {
                form: { isDirty: true, processing: false, errors: {} },
                warnUnsaved: true,
              })
            : null,
        ],
      }),
  }).use(createPinia())
  installPresentation(app, shell && { id: 'topbar', apiVersion: 1, adminShell: shell })
  app.mount(root)
  return root
}

const triggerSelector = 'button[data-quick-navigation-trigger]'
const dialog = () => document.querySelector<HTMLElement>('[data-quick-navigation]')
const search = () => dialog()!.querySelector<HTMLInputElement>('[role="combobox"]')!
const options = () => Array.from(dialog()!.querySelectorAll<HTMLElement>('[role="option"]'))
const paths = () => options().map((option) => option.querySelector('span:last-child')!.textContent)
const key = (target: EventTarget, key: string, init: KeyboardEventInit = {}) => {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init })
  target.dispatchEvent(event)
  return event
}
const settle = async () => {
  await nextTick()
  await nextTick()
}

async function filter(query: string) {
  search().value = query
  search().dispatchEvent(new Event('input', { bubbles: true }))
  await settle()
}

describe('quick navigation', () => {
  it.each([
    ['sidebar', undefined],
    ['topbar', TopbarShell],
  ] as const)('renders a single trigger and dialog in the %s shell', async (name, shell) => {
    const root = mount(shell)
    expect(root.querySelector('[data-ui="admin-shell"]')?.getAttribute('data-part')).toBe(name)
    const triggers = root.querySelectorAll<HTMLButtonElement>(triggerSelector)
    expect(triggers).toHaveLength(1)
    const trigger = triggers[0]!
    trigger.focus()
    trigger.click()
    await settle()
    expect(document.querySelectorAll('[data-quick-navigation]')).toHaveLength(1)
    expect(dialog()?.getAttribute('role')).toBe('dialog')
    expect(dialog()?.getAttribute('aria-modal')).toBe('true')
    expect(document.activeElement).toBe(search())
    key(search(), 'Escape')
    await vi.waitFor(() => expect(dialog()).toBeNull())
    await vi.waitFor(() => expect(document.activeElement).toBe(trigger))
  })

  it('searches nested translated labels and paths from the current permitted menu', async () => {
    const root = mount()
    root.querySelector<HTMLButtonElement>(triggerSelector)!.click()
    await settle()
    expect(paths()).toEqual([
      'Доступ · Permissions group · /admins',
      'Доступ · /roles',
      'Доступ · Роли · /roles/permissions',
      '/auth/profile',
      '/auth/profile/settings',
      '/auth/profile/settings#connection',
    ])

    await filter('разрешения /roles')
    expect(paths()).toEqual(['Доступ · Роли · /roles/permissions'])
    applyLocale('en')
    await settle()
    expect(paths()).toEqual([])
    expect(search().hasAttribute('aria-activedescendant')).toBe(false)
    expect(dialog()!.querySelector('[role="status"]')).not.toBeNull()
    await filter('ADMINISTRATORS /admins')
    expect(paths()).toHaveLength(1)

    page.props.adminMenu.sections = [{ key: 'main', items: [{ name: 'Главная', href: '/' }] }]
    await settle()
    expect(paths()).toEqual([])
    await filter('')
    expect(paths()).toEqual(['/', '/auth/profile', '/auth/profile/settings'])
  })

  it('handles Russian and English shortcuts, tracks selection with arrows, and cleans up', async () => {
    const get = vi.spyOn(router, 'get').mockImplementation((_, __, options) => {
      options?.onStart?.({} as Parameters<NonNullable<typeof options.onStart>>[0])
    })
    const root = mount()
    const opener = root.querySelector<HTMLButtonElement>('#opener')!
    opener.focus()
    expect(key(opener, 'л', { code: 'KeyK', ctrlKey: true }).defaultPrevented).toBe(true)
    await settle()
    const input = search()
    expect(document.activeElement).toBe(input)
    const selected = () => document.getElementById(input.getAttribute('aria-activedescendant')!)
    expect(selected()).toBe(options()[0])
    key(input, 'ArrowUp')
    await settle()
    expect(selected()).toBe(options().at(-1))
    key(input, 'ArrowDown')
    key(input, 'ArrowDown')
    await settle()
    expect(selected()).toBe(options()[1])
    expect(selected()?.getAttribute('aria-selected')).toBe('true')
    key(input, 'Enter')
    expect(get).toHaveBeenCalledWith(
      '/roles',
      {},
      expect.objectContaining({ onStart: expect.any(Function) }),
    )
    await vi.waitFor(() => expect(dialog()).toBeNull())
    await vi.waitFor(() => expect(document.activeElement).toBe(opener))

    expect(key(opener, 'k', { metaKey: true }).defaultPrevented).toBe(true)
    await settle()
    expect(search().getAttribute('aria-activedescendant')).toBe(options()[0]!.id)
    key(search(), 'ArrowUp')
    key(search(), 'Escape')
    await vi.waitFor(() => expect(dialog()).toBeNull())
    await vi.waitFor(() => expect(document.activeElement).toBe(opener))
    key(opener, 'k', { ctrlKey: true })
    await settle()
    expect(search().getAttribute('aria-activedescendant')).toBe(options()[0]!.id)
    app!.unmount()
    app = undefined
    expect(key(document.body, 'k', { ctrlKey: true }).defaultPrevented).toBe(false)
  })

  it('leaves editing shortcuts and active dialogs alone', async () => {
    const root = mount()
    const editor = root.querySelector<HTMLTextAreaElement>('#editor')!
    const child = root.querySelector<HTMLElement>('#editable-child')!
    editor.focus()
    expect(key(editor, 'k', { ctrlKey: true }).defaultPrevented).toBe(false)
    editor.blur()
    expect(key(child, 'л', { ctrlKey: true }).defaultPrevented).toBe(false)
    const otherDialog = document.createElement('div')
    otherDialog.setAttribute('role', 'dialog')
    document.body.append(otherDialog)
    expect(key(document.body, 'k', { ctrlKey: true }).defaultPrevented).toBe(false)
    await settle()
    expect(dialog()).toBeNull()
  })

  it('runs the actual Inertia unsaved-form guard before committing navigation', async () => {
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const root = mount(undefined, true)
    root.querySelector<HTMLButtonElement>(triggerSelector)!.click()
    await settle()
    await filter('/roles')
    options()[0]!.click()
    expect(confirm).toHaveBeenCalledWith('Есть несохранённые изменения. Покинуть страницу?')
    expect(dialog()).not.toBeNull()
  })
})
