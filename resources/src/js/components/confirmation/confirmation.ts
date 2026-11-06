import {
  inject,
  nextTick,
  onScopeDispose,
  provide,
  shallowRef,
  watch,
  type InjectionKey,
} from 'vue'
import { router, usePage } from '@inertiajs/vue3'
import { appearance } from '@/appearance/state'

export interface ConfirmationOptions {
  message: string
  params?: Record<string, string | number>
  title?: string
  confirmLabel?: string
  variant?: 'primary' | 'danger'
}

export function createConfirmation() {
  const active = shallowRef<(ConfirmationOptions & { id: number }) | null>(null)
  let sequence = 0
  let generation = 0
  let pending: { id: number; owner: symbol; resolve: (accepted: boolean) => void } | undefined
  let trigger: HTMLElement | null = null
  function settle(id: number, accepted: boolean) {
    if (!pending || pending.id !== id) return
    const resolve = pending.resolve
    const previous = trigger
    pending = undefined
    trigger = null
    active.value = null
    resolve(accepted)
    void nextTick(() => {
      if (previous?.isConnected) previous.focus()
    })
  }
  function cancel(owner?: symbol) {
    if (owner && pending?.owner !== owner) return
    generation++
    if (pending) settle(pending.id, false)
  }
  async function request(options: ConfirmationOptions, owner: symbol) {
    if (pending) return false
    const epoch = generation
    const id = ++sequence
    trigger =
      typeof document !== 'undefined' && document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    const answer = new Promise<boolean>((resolve) => {
      pending = { id, owner, resolve }
    })
    active.value = { ...options, id }
    const accepted = await answer
    return accepted && epoch === generation
  }
  return { active, request, settle, cancel, epoch: () => generation }
}

export type ConfirmationController = ReturnType<typeof createConfirmation>
const confirmationKey: InjectionKey<ConfirmationController> = Symbol('admin-confirmation')

export function provideConfirmation(controller = createConfirmation()) {
  provide(confirmationKey, controller)
  const cancelNavigation = () => controller.cancel()
  const stopNavigation = router.on('start', cancelNavigation)
  // History restoration and client-side visits need not emit a request start.
  const page = usePage()
  const stopPage = watch(() => page.props, cancelNavigation, { flush: 'sync' })
  if (typeof window !== 'undefined') window.addEventListener('popstate', cancelNavigation)
  const stopOwner = watch(
    () => `${appearance.owner}:${appearance.readOnly}:${appearance.resolving}`,
    () => controller.cancel(),
    { flush: 'sync' },
  )
  onScopeDispose(() => {
    controller.cancel()
    stopNavigation()
    stopPage()
    stopOwner()
    if (typeof window !== 'undefined') window.removeEventListener('popstate', cancelNavigation)
  })
  return controller
}

export function useConfirmation() {
  const controller = inject(confirmationKey, null)
  const owner = Symbol('confirmation-requester')
  let alive = true
  onScopeDispose(() => {
    alive = false
    controller?.cancel(owner)
  })
  return async (
    options: ConfirmationOptions,
    action: () => void | Promise<void>,
  ): Promise<boolean> => {
    if (!controller || !alive) return false
    const epoch = controller.epoch()
    const accepted = await controller.request(options, owner)
    if (!alive || !accepted || epoch !== controller.epoch()) return false
    // Dispatch in this continuation, with no await between the fence and the
    // action. Returning a boolean for callers to await leaves another race gap.
    await action()
    return true
  }
}
