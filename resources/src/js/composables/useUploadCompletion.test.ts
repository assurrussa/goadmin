import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'

const completion = {
  location: '/files/tus/original',
  filename: 'clip.mp4',
  fileCategory: 'video',
  context: 'rich-text',
  entityType: 'article',
  entityId: 7,
}
const load = async (id = 7, endpoint?: string) => {
  const { useUploadCompletion } = await import('./useUploadCompletion')
  return useUploadCompletion(
    () => 'article',
    () => id,
    () => 'rich-text',
    () => endpoint,
  )
}
beforeEach(() => {
  vi.resetModules()
  setActivePinia(createPinia())
  sessionStorage.clear()
  vi.restoreAllMocks()
})

describe('durable upload completion fence', () => {
  it('shares a fence across remounts and restores only metadata after module reload', async () => {
    const first = await load()
    const file = new File(['bytes'], completion.filename, { type: 'video/mp4' })
    first.retain(completion, file)
    expect((await load()).pending.value).toEqual({ completion, file })
    expect(sessionStorage.length).toBe(1)
    expect(sessionStorage.getItem(sessionStorage.key(0)!)).not.toContain('bytes')
    vi.resetModules()
    const restored = await load()
    expect(restored.pending.value).toEqual({ completion })
    restored.release(completion)
    expect(restored.pending.value).toBeNull()
    expect(sessionStorage.length).toBe(0)
  })

  it('isolates entities, endpoint namespaces and authenticated users', async () => {
    const first = await load()
    first.retain(completion)
    expect((await load(8)).pending.value).toBeNull()
    expect((await load(7, '/other/files')).pending.value).toBeNull()
    const { useAuthUserStore } = await import('@/stores/authUser')
    const auth = useAuthUserStore()
    auth.setAuthUser({
      id: 'other-user',
      name: '',
      lastName: '',
      email: '',
      roles: [],
      avatarUrl: '',
    })
    expect(first.pending.value).toBeNull()
    expect((await load()).pending.value).toBeNull()
    auth.setAuthUser(null)
    expect(first.pending.value?.completion).toEqual(completion)
  })

  it('does not overwrite or release a different unresolved session', async () => {
    const state = await load()
    state.retain(completion)
    const different = { ...completion, location: '/files/tus/new-session' }
    expect(state.retain(different)).toBe(false)
    state.release(different)
    expect(state.pending.value?.completion).toEqual(completion)
  })

  it('ignores malformed or wrong-entity persisted metadata', async () => {
    const state = await load()
    state.retain(completion)
    const key = sessionStorage.key(0)!
    sessionStorage.setItem(key, JSON.stringify({ ...completion, entityId: 8 }))
    vi.resetModules()
    expect((await load()).pending.value).toBeNull()
    sessionStorage.setItem(key, '{')
    vi.resetModules()
    expect((await load()).pending.value).toBeNull()
  })

  it('keeps remounts fenced when storage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('disabled')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('disabled')
    })
    const first = await load()
    first.retain(completion)
    const remounted = await load()
    expect(remounted.pending.value?.completion).toEqual(completion)
  })
  it('does not resurrect uncertainty from a stale response after a definitive replay', async () => {
    const first = await load()
    first.retain(completion)
    const remounted = await load()
    remounted.release(completion)
    first.retain(completion)
    expect(remounted.pending.value).toBeNull()
    expect(sessionStorage.length).toBe(0)
  })
  it('disables a stale owner/endpoint/context scope before another dispatch', async () => {
    const { useUploadCompletion } = await import('./useUploadCompletion')
    const endpoint = ref('/files')
    const context = ref('rich-text')
    const state = useUploadCompletion(
      () => 'article',
      () => 7,
      () => context.value,
      () => endpoint.value,
    )
    state.retain(completion)
    endpoint.value = '/other/files'
    expect(state.isCurrentScope.value).toBe(false)
    expect(state.pending.value).toBeNull()
    endpoint.value = '/files'
    expect(state.pending.value?.completion).toEqual(completion)
    context.value = 'another-widget'
    expect(state.isCurrentScope.value).toBe(false)
    expect(state.pending.value).toBeNull()
  })
  it('persists separate stable fields without changing the server upload context', async () => {
    let { useUploadCompletion } = await import('./useUploadCompletion')
    const first = useUploadCompletion(
      () => 'article',
      () => 7,
      () => 'rich-text',
      undefined,
      () => 'body',
    )
    const second = useUploadCompletion(
      () => 'article',
      () => 7,
      () => 'rich-text',
      undefined,
      () => 'summary',
    )
    first.retain(completion)
    expect(second.pending.value).toBeNull()
    vi.resetModules()
    ;({ useUploadCompletion } = await import('./useUploadCompletion'))
    const restored = useUploadCompletion(
      () => 'article',
      () => 7,
      () => 'rich-text',
      undefined,
      () => 'body',
    )
    expect(restored.pending.value?.completion).toEqual(completion)
    restored.release(completion)
  })
})
