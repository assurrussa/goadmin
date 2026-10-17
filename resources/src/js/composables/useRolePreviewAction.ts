import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'

// This is presentation only. The server checks current canonical permissions
// and refuses every preview mutation independently of links and browser props.
export function useRolePreviewAction(domain: string, action: string) {
  const page = usePage()
  return computed(() => {
    const preview = page.props.rolePreview as
      | { active?: boolean; actions?: Record<string, unknown> }
      | undefined
    if (!preview?.active) return true
    return preview.actions?.[`${domain}:${action}`] === true
  })
}
