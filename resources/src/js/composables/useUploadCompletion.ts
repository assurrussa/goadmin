import { computed, shallowRef, type ShallowRef } from 'vue'
import { resolveTusEndpoint } from '@/services/tusEndpoint'
import { useAuthUserStore } from '@/stores/authUser'
import type { UncertainUploadCompletion } from '@/services/fileUploadService'

export interface PendingUploadCompletion {
  completion: UncertainUploadCompletion
  // Keep bytes only in memory. A replay needs metadata, never the original file.
  file?: File
}

const states = new Map<string, ShallowRef<PendingUploadCompletion | null>>()
const resolvedSessions = new Set<string>()
const storageKey = (
  owner: string | null,
  endpoint: string,
  entityType: string,
  entityId: unknown,
  context?: string,
) =>
  `goadmin:upload-completion:${JSON.stringify([owner, endpoint, entityType, String(entityId), context ?? ''])}`

const stateFor = (key: string): ShallowRef<PendingUploadCompletion | null> => {
  let state = states.get(key)
  if (!state) {
    let restored: PendingUploadCompletion | null = null
    try {
      const data = JSON.parse(sessionStorage.getItem(key) ?? 'null')
      if (
        data &&
        typeof data.location === 'string' &&
        typeof data.filename === 'string' &&
        typeof data.entityType === 'string' &&
        ['number', 'string'].includes(typeof data.entityId) &&
        JSON.stringify([data.entityType, String(data.entityId), data.context ?? '']) ===
          JSON.stringify(JSON.parse(key.slice('goadmin:upload-completion:'.length)).slice(2))
      ) {
        restored = { completion: data }
      }
    } catch {
      // Storage may be unavailable; the shared in-memory fence still survives remounts.
    }
    state = shallowRef(restored)
    states.set(key, state)
  }
  return state
}

/** Same-tab completion fence. Persist before dispatch, release only after a definite result. */
export function useUploadCompletion(
  entityType: () => string,
  entityId: () => string | number | null | undefined,
  context: () => string | undefined = () => undefined,
  uploadUrl: () => string | undefined = () => undefined,
) {
  const auth = useAuthUserStore()
  const owner = auth.id
  const endpoint = resolveTusEndpoint(uploadUrl())
  const initialContext = context()
  const keyFor = (completion: UncertainUploadCompletion) =>
    storageKey(
      owner,
      endpoint,
      completion.entityType,
      completion.entityId,
      completion.context ?? initialContext,
    )
  const isCurrentScope = computed(
    () =>
      auth.id === owner &&
      resolveTusEndpoint(uploadUrl()) === endpoint &&
      context() === initialContext,
  )
  const pending = computed(() =>
    isCurrentScope.value
      ? stateFor(storageKey(owner, endpoint, entityType(), entityId(), context())).value
      : null,
  )

  const retain = (completion: UncertainUploadCompletion, file?: File) => {
    const key = keyFor(completion)
    if (resolvedSessions.has(JSON.stringify([key, completion.location]))) return false
    const state = stateFor(key)
    // An unresolved session must never be replaced by a different one.
    if (state.value && state.value.completion.location !== completion.location) return false
    state.value = { completion, file: file ?? state.value?.file }
    try {
      sessionStorage.setItem(
        key,
        JSON.stringify({ ...completion, context: completion.context ?? initialContext }),
      )
    } catch {
      // Navigation within the application remains fenced without browser storage.
    }
    return true
  }

  const release = (completion: UncertainUploadCompletion) => {
    const key = keyFor(completion)
    const state = stateFor(key)
    if (state.value?.completion.location !== completion.location) return
    resolvedSessions.add(JSON.stringify([key, completion.location]))
    state.value = null
    try {
      sessionStorage.removeItem(key)
    } catch {
      // No storage access; only the shared in-memory state is available.
    }
  }

  return { pending, retain, release, isCurrentScope }
}
