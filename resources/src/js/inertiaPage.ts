import type { Component, DefineComponent, VNode, h } from 'vue'

export type LayoutResolver = (render: typeof h, page: VNode) => VNode
export type PageWithLayout = Component & {
  layout?: Component | Component[] | LayoutResolver
}
export type PageModule = { default: PageWithLayout }
export type PageLoader = () => Promise<PageModule>

// Inertia 2's Vue resolver declares bare DefineComponent, whose default props
// are invariant with SFCs declaring required props. At runtime Inertia passes
// the resolved Vue component unchanged to h(component, page.props). Accept Vue's
// actual renderable Component contract here, and adapt only this upstream type
// boundary. Do not apply this assertion to host imports or their prop types.
export const toInertiaPage = (page: PageWithLayout): DefineComponent => page as DefineComponent
