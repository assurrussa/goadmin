import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, ref } from 'vue'
import { applyLocale } from '@/i18n'
import { appearance } from '@/appearance/state'
import ConfirmationDialog from './ConfirmationDialog.vue'
import {
  createConfirmation,
  provideConfirmation,
  useConfirmation,
  type ConfirmationController,
  type ConfirmationOptions,
} from './confirmation'

const navigation = vi.hoisted(() => ({
  start: undefined as (() => void) | undefined,
  stop: vi.fn(),
  page: undefined as unknown as { props: Record<string, unknown> },
}))
vi.mock('@inertiajs/vue3', () => ({
  usePage: () => navigation.page,
  router: {
    on: (_name: string, callback: () => void) => {
      navigation.start = callback
      return navigation.stop
    },
  },
}))
let app: ReturnType<typeof createApp> | undefined
const settle = async () => {
  for (let i = 0; i < 8; i++) await nextTick()
}
beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  )
  navigation.stop.mockClear()
  navigation.page = reactive({ props: {} })
  appearance.owner = 'actor-1'
  appearance.readOnly = false
  appearance.resolving = false
  applyLocale('ru')
})
afterEach(async () => {
  app?.unmount()
  app = undefined
  await settle()
  document.body.innerHTML = ''
  applyLocale('ru')
  vi.unstubAllGlobals()
})
function mount(
  options: ConfirmationOptions = {
    message: 'Вы уверены, что хотите удалить запись?',
    confirmLabel: 'Удалить',
    variant: 'danger',
  },
) {
  let controller!: ConfirmationController
  const visible = ref(true)
  const answers: boolean[] = []
  let ask!: ReturnType<typeof useConfirmation>
  const Requester = defineComponent({
    setup() {
      ask = useConfirmation()
      return () =>
        h(
          'button',
          {
            id: 'trigger',
            onClick: () => {
              void ask(options, () => {}).then((answer) => answers.push(answer))
            },
          },
          'Action',
        )
    },
  })
  const root = document.createElement('div')
  document.body.append(root)
  app = createApp({
    setup() {
      controller = provideConfirmation()
      return () => [visible.value ? h(Requester) : null, h(ConfirmationDialog, { controller })]
    },
  })
  app.mount(root)
  return {
    controller,
    visible,
    answers,
    ask: (value: ConfirmationOptions, action: () => void = () => {}) => ask(value, action),
    trigger: root.querySelector<HTMLButtonElement>('button')!,
  }
}

it('renders a localized destructive dialog, focuses Cancel and settles acceptance once', async () => {
  const { trigger, answers } = mount()
  trigger.focus()
  trigger.click()
  await settle()
  const dialog = document.querySelector<HTMLElement>('.goadmin-confirmation-dialog')!
  expect(dialog.getAttribute('role')).toBe('dialog')
  expect(dialog.getAttribute('aria-modal')).toBe('true')
  expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toContain(
    'Подтвердите действие',
  )
  expect(document.getElementById(dialog.getAttribute('aria-describedby')!)?.textContent).toContain(
    'Вы уверены, что хотите удалить запись?',
  )
  expect(dialog.textContent).toContain('Подтвердите действие')
  expect(dialog.textContent).toContain('Вы уверены, что хотите удалить запись?')
  const cancel = dialog.querySelector<HTMLButtonElement>('[data-confirmation-cancel]')!
  const accept = dialog.querySelector<HTMLButtonElement>('[data-confirmation-accept]')!
  expect(document.activeElement).toBe(cancel)
  expect(accept.textContent).toBe('Удалить')
  expect(accept.classList.contains('bg-destructive')).toBe(true)
  expect(accept.classList.contains('goadmin-primary-fill')).toBe(false)
  expect(answers).toEqual([])
  accept.click()
  accept.click()
  await settle()
  expect(answers).toEqual([true])
  expect(document.querySelector('.goadmin-confirmation-dialog')).toBeNull()
})

it('cancels with Escape, restores focus and translates labels and message on locale changes', async () => {
  const { trigger, answers } = mount()
  trigger.focus()
  trigger.click()
  await settle()
  applyLocale('en')
  await settle()
  const dialog = document.querySelector('.goadmin-confirmation-dialog')!
  expect(dialog.textContent).toContain('Confirm action')
  expect(dialog.textContent).toContain('Are you sure you want to delete the record?')
  expect(dialog.querySelector('[data-confirmation-cancel]')?.textContent).toBe('Cancel')
  expect(dialog.querySelector('[data-confirmation-accept]')?.textContent).toBe('Delete')
  document.activeElement!.dispatchEvent(
    new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
  )
  await settle()
  expect(answers).toEqual([false])
  expect(document.activeElement).toBe(trigger)
})

it.each(['navigation', 'history', 'page', 'actor', 'requester'] as const)(
  'cancels pending work on %s',
  async (reason) => {
    const { trigger, answers, visible } = mount()
    trigger.click()
    await settle()
    if (reason === 'navigation') navigation.start!()
    else if (reason === 'history') window.dispatchEvent(new PopStateEvent('popstate'))
    else if (reason === 'page') navigation.page.props = { title: 'Next page' }
    else if (reason === 'actor') appearance.owner = 'actor-2'
    else visible.value = false
    await settle()
    expect(answers).toEqual([false])
    expect(document.querySelector('.goadmin-confirmation-dialog')).toBeNull()
  },
)

it('fails closed without an application provider', async () => {
  let answer!: Promise<boolean>
  app = createApp({
    setup() {
      const confirm = useConfirmation()
      answer = confirm({ message: 'Delete?' }, () => {})
      return () => null
    },
  })
  app.mount(document.createElement('div'))
  expect(await answer).toBe(false)
})

it('rejects overlapping prompts and fences an accepted result cancelled before its continuation', async () => {
  const controller = createConfirmation()
  const first = controller.request({ message: 'First' }, Symbol())
  expect(await controller.request({ message: 'Second' }, Symbol())).toBe(false)
  controller.settle(controller.active.value!.id, true)
  controller.cancel()
  expect(await first).toBe(false)
})

it('ignores a late click from the previous dialog when another request opens', async () => {
  const { trigger, controller, ask } = mount()
  trigger.click()
  await settle()
  const previousAccept = document.querySelector<HTMLButtonElement>('[data-confirmation-accept]')!
  previousAccept.click()
  const nextAnswer = ask({ message: 'Next action' })
  previousAccept.click()
  await settle()
  expect(controller.active.value?.message).toBe('Next action')
  controller.settle(controller.active.value!.id, false)
  expect(await nextAnswer).toBe(false)
})

it('cancels with the close button without dispatching and restores the opener', async () => {
  const { trigger, ask } = mount()
  const action = vi.fn()
  trigger.focus()
  const answer = ask({ message: 'Delete?' }, action)
  await settle()
  document.querySelector<HTMLButtonElement>('[data-ui="dialog"][data-part="close"]')!.click()
  await settle()
  expect(await answer).toBe(false)
  expect(action).not.toHaveBeenCalled()
  expect(document.querySelector('.goadmin-confirmation-dialog')).toBeNull()
  expect(document.activeElement).toBe(trigger)
})

it('keeps focus in a consecutive dialog and returns to the surviving original opener', async () => {
  const { trigger, ask, controller } = mount()
  trigger.focus()
  const first = ask({ message: 'First action' })
  await settle()
  const firstAccept = document.querySelector<HTMLButtonElement>('[data-confirmation-accept]')!
  firstAccept.focus()
  firstAccept.click()
  const second = ask({ message: 'Second action' })
  await settle()
  await new Promise((resolve) => setTimeout(resolve, 0))
  const dialog = document.querySelector<HTMLElement>('.goadmin-confirmation-dialog')!
  expect(dialog.contains(document.activeElement)).toBe(true)
  expect(await first).toBe(true)
  controller.settle(controller.active.value!.id, false)
  expect(await second).toBe(false)
  await settle()
  await new Promise((resolve) => setTimeout(resolve, 0))
  expect(document.activeElement).toBe(trigger)
})

it.each(['navigation', 'actor', 'history'] as const)(
  'does not dispatch after %s changes between promise continuations',
  async (reason) => {
    const { controller, ask } = mount()
    const mutate = vi.fn()
    const answer = ask({ message: 'Delete?' }, mutate)
    controller.settle(controller.active.value!.id, true)
    queueMicrotask(() => {
      if (reason === 'navigation') navigation.start!()
      else if (reason === 'actor') appearance.owner = 'actor-2'
      else window.dispatchEvent(new PopStateEvent('popstate'))
    })
    expect(await answer).toBe(false)
    expect(mutate).not.toHaveBeenCalled()
  },
)
