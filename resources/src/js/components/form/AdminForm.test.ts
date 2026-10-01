import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, reactive } from 'vue'
import AdminForm from './AdminForm.vue'

const mocks = vi.hoisted(() => ({
  on: vi.fn(),
  remove: vi.fn(),
}))

vi.mock('@inertiajs/vue3', () => ({
  usePage: () => ({ props: { errors: {} } }),
  router: { on: mocks.on },
}))

afterEach(() => {
  vi.unstubAllGlobals()
  mocks.on.mockReset()
  mocks.remove.mockReset()
})

describe('AdminForm unsaved changes guard', () => {
  it('confirms only dirty GET navigation and removes the subscription on unmount', () => {
    mocks.on.mockReturnValue(mocks.remove)
    const confirm = vi.fn(() => false)
    vi.stubGlobal('confirm', confirm)
    const form = reactive({ errors: {}, processing: false, isDirty: true })
    const app = createApp(AdminForm, { form, warnUnsaved: true })
    app.mount(document.createElement('div'))

    expect(mocks.on).toHaveBeenCalledWith('before', expect.any(Function))
    const guard = mocks.on.mock.calls[0][1] as (event: {
      detail: { visit: { method: string } }
    }) => boolean | void
    expect(guard({ detail: { visit: { method: 'get' } } })).toBe(false)
    expect(confirm).toHaveBeenCalledTimes(1)
    expect(guard({ detail: { visit: { method: 'post' } } })).toBeUndefined()
    form.processing = true
    expect(guard({ detail: { visit: { method: 'get' } } })).toBeUndefined()
    expect(confirm).toHaveBeenCalledTimes(1)

    app.unmount()
    expect(mocks.remove).toHaveBeenCalledOnce()
  })
})
