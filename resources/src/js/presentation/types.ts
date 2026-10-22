import type { HTMLAttributes, VNode } from 'vue'

export type AdminShellProps = { collapsed: boolean }
export type NavigationOptions = { orientation: 'vertical' | 'horizontal'; collapsed?: boolean }
type Content = () => VNode[]
export type AdminShellSlots = {
  default: Content
  header: Content
  navigation: (options: NavigationOptions) => VNode[]
  mobileNavigation: Content
  navigationToggle: Content
  account: Content
  utilities: Content
  breadcrumbs: Content
  brand: Content
}
export type AuthShellProps = { class?: HTMLAttributes['class'] }
export type AuthShellSlots = {
  default: Content
  title: Content
  subtitle?: Content
  footer?: Content
  brand: Content
  utilities: Content
}
export type AdminShellComponent = new () => {
  $props: AdminShellProps
  $slots: AdminShellSlots
}
export type AuthShellComponent = new () => {
  $props: AuthShellProps
  $slots: AuthShellSlots
}
export type AdminThemePack = {
  id: string
  apiVersion: 1
  adminShell?: AdminShellComponent
  authShell?: AuthShellComponent
}

type PropsOf<C> = C extends new () => { $props: infer P } ? P : never
type CheckShell<T, K extends PropertyKey, P> = K extends keyof T
  ? P extends PropsOf<NonNullable<T[K]>>
    ? unknown
    : { [Name in K]: never }
  : unknown

/** A shell may accept optional props, but cannot require data the core never supplies. */
export function defineAdminTheme<T extends AdminThemePack>(
  theme: T &
    CheckShell<T, 'adminShell', AdminShellProps> &
    CheckShell<T, 'authShell', AuthShellProps>,
): T {
  return theme
}
