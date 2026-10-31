// Inertia writes history.state.page for fresh visits as well as restorations.
// Track navigation provenance before asynchronous page resolution mounts a grid.
let restoring = false
if (typeof window !== 'undefined') {
  restoring =
    window.performance
      ?.getEntriesByType?.('navigation')
      .some((entry) => (entry as PerformanceNavigationTiming).type === 'back_forward') ?? false
  window.addEventListener('popstate', () => {
    restoring = true
  })
  window.addEventListener('pageshow', (event) => {
    if (event.persisted) restoring = true
  })
  document.addEventListener('inertia:before', () => {
    restoring = false
  })
}
export const isGridHistoryRestoration = () => restoring
