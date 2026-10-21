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
  field = '',
) =>
  `goadmin:upload-completion:${JSON.stringify([owner, endpoint, entityType, String(entityId), context ?? '', field])}`

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
          JSON.stringify(JSON.parse(key.slice('goadmin:upload-completion:'.length)).slice(2, 5))
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
  field: () => string | undefined = () => undefined,
  recoverLegacy = false,
) {
  const auth = useAuthUserStore()
  const owner = auth.id
  const endpoint = resolveTusEndpoint(uploadUrl())
  const initialContext = context()
  const initialField = field() ?? ''
  const keyFor = (completion: UncertainUploadCompletion, field = initialField) =>
    storageKey(
      owner,
      endpoint,
      completion.entityType,
      completion.entityId,
      completion.context ?? initialContext,
      field,
    )
  const isCurrentScope = computed(
    () =>
      auth.id === owner &&
      resolveTusEndpoint(uploadUrl()) === endpoint &&
      context() === initialContext &&
      (field() ?? '') === initialField,
  )
  // A newly named field must not hide a session retained by an older UI.
  // Its field ownership is still unknown, so recovery requires explicit assignment.
  const legacyPending = computed(() =>
    isCurrentScope.value && initialField && recoverLegacy
      ? stateFor(storageKey(owner, endpoint, entityType(), entityId(), context(), '')).value
      : null,
  )
  const pending = computed(() => {
    if (!isCurrentScope.value) return null
    return (
      legacyPending.value ??
      stateFor(storageKey(owner, endpoint, entityType(), entityId(), context(), initialField)).value
    )
  })
  const pendingFieldKnown = computed(() => !!initialField && !legacyPending.value)

  const retain = (completion: UncertainUploadCompletion, file?: File) => {
    if (
      initialField &&
      recoverLegacy &&
      resolvedSessions.has(JSON.stringify([keyFor(completion, ''), completion.location]))
    ) {
      return false
    }
    const legacy = initialField && recoverLegacy ? stateFor(keyFor(completion, '')).value : null
    if (legacy && legacy.completion.location !== completion.location) return false
    const key = keyFor(completion, legacy ? '' : initialField)
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
    const legacy = initialField && recoverLegacy ? stateFor(keyFor(completion, '')).value : null
    const key = keyFor(
      completion,
      legacy?.completion.location === completion.location ? '' : initialField,
    )
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

  return { pending, pendingFieldKnown, retain, release, isCurrentScope }
}
