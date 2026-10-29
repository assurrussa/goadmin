import { watch } from 'vue'
import { sessionCoordinator } from './index'

/** Keep page-owned commands separate from the document-owned session watchdog. */
export function installSessionNavigationCancellation(current: {
  component: string
  url: string
}): () => void {
  const withoutHash = (value: string) => {
    const url = new URL(value, window.location.origin)
    url.hash = ''
    return url.href
  }
  // The Vue adapter updates usePage during swap, after lazy resolution/history
  // decryption and before destination setup/mount. A synchronous watcher fences
  // every command from the old page, including those started during chunk load.
  return watch(
    () => [current.component, withoutHash(current.url)] as const,
    (next, previous) => {
      if (previous[0] && (next[0] !== previous[0] || next[1] !== previous[1]))
        sessionCoordinator.cancel()
    },
    { flush: 'sync' },
  )
}
