import { router, usePage, type InertiaForm } from '@inertiajs/vue3'
import { v4 as uuid } from 'uuid'
import { nextTick, onScopeDispose, watch, type Ref } from 'vue'

interface RecordEditFields {
  name: string
  lastName: string | null
  email: string
  username: string | null
}

function ownerId(props: Record<string, unknown>): string {
  const actor = props.authuser as { id?: string | number; idExact?: string } | undefined
  return String(actor?.idExact || actor?.id || '')
}

export function useRecordEditSubmission(
  form: InertiaForm<RecordEditFields>,
  basePath: string,
  listHref: Ref<string>,
  recordId: () => string | number | undefined,
  canonicalFields: () => RecordEditFields,
) {
  const page = usePage()
  const currentOwner = () => ownerId(page.props)
  let revision = 0
  let saving = false

  watch([recordId, currentOwner], () => {
    revision++
    // A preserved Inertia component can receive another record or actor.
    form.defaults(canonicalFields()).reset().clearErrors()
  })
  onScopeDispose(() => revision++)

  return function submit(_event: Event, returnToList = false) {
    const id = recordId()
    if (saving || form.processing || id == null) return
    const attempt = revision
    const owner = currentOwner()
    const component = page.component
    const destination = returnToList ? listHref.value : undefined
    const submitted = form.data()
    const requestId = uuid()
    const stillCurrent = () =>
      attempt === revision &&
      recordId() === id &&
      currentOwner() === owner &&
      page.component === component
    let confirmed = false

    form.put(`${basePath}/${id}`, {
      headers: { 'X-GoAdmin-Record-Attempt': requestId },
      onStart: () => {
        saving = true
      },
      onSuccess: (response) => {
        const record = response.props.data as { id?: string | number } | undefined
        const flash = response.props.flash as
          | { success?: string; error?: string; recordEditAttempt?: string }
          | undefined
        if (
          !stillCurrent() ||
          response.component !== component ||
          record?.id !== id ||
          ownerId(response.props) !== owner
        ) {
          // Preserve the existing baseline when Inertia received a page without a save result.
          form.defaults({})
          return
        }
        if (!flash?.success || flash.error || flash.recordEditAttempt !== requestId) {
          form.defaults({})
          const errors: Record<string, string> = {
            form: 'Результат операции неизвестен. Обновите страницу перед повтором.',
          }
          form.setError(errors)
          return
        }

        // A pending email change is saved against the still-canonical address.
        const email = canonicalFields().email
        if (form.email === submitted.email) form.email = email
        form.defaults({ ...submitted, email })
        confirmed = true
      },
      onFinish: async (visit) => {
        // Let Vue apply the final draft and settle Inertia's dirty check before navigation.
        await nextTick()
        saving = false
        if (
          destination &&
          owner &&
          confirmed &&
          !visit.cancelled &&
          !visit.interrupted &&
          stillCurrent() &&
          !form.isDirty
        )
          router.get(destination)
      },
    })
  }
}
