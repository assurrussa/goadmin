import { computed, shallowRef } from 'vue'

export type AdminCapability = 'realtime' | 'notifications' | 'uploads' | 'authmail'
export type AdminCapabilities = Partial<Record<AdminCapability, boolean>> & Record<string, boolean>

const capabilities = shallowRef<AdminCapabilities | undefined>(undefined)

// Older embedding hosts omit this prop and retain their existing full profile.
// An explicitly present map enables only capabilities set to true.
export function setAdminCapabilities(pageLike: unknown): void {
  const props = (pageLike as { props?: Record<string, unknown> } | null)?.props
  if (!props || !Object.prototype.hasOwnProperty.call(props, 'adminCapabilities')) {
    capabilities.value = undefined
    return
  }
  const value = props.adminCapabilities
  capabilities.value =
    typeof value === 'object' && value !== null && !Array.isArray(value)
      ? (value as AdminCapabilities)
      : {}
}

export function hasAdminCapability(capability: AdminCapability): boolean {
  return capabilities.value === undefined || capabilities.value[capability] === true
}

export function useAdminCapabilities() {
  return {
    realtime: computed(() => hasAdminCapability('realtime')),
    notifications: computed(() => hasAdminCapability('notifications')),
    uploads: computed(() => hasAdminCapability('uploads')),
    authmail: computed(() => hasAdminCapability('authmail')),
  }
}
