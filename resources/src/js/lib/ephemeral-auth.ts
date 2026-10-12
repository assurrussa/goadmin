// One-way lifecycle fence for transient authentication material. A restored
// document reloads authoritative state; old async responses never reactivate it.
export function bindEphemeralAuthPage(clear: () => void, reload = () => window.location.reload()) {
 let active = true
 const leave = () => { active = false; clear() }
 const restore = (event: PageTransitionEvent) => { if (event.persisted) reload() }
 window.addEventListener('pagehide', leave)
 window.addEventListener('pageshow', restore)
 return {
  isActive: () => active,
  dispose: () => {
   leave()
   window.removeEventListener('pagehide', leave)
   window.removeEventListener('pageshow', restore)
  },
 }
}
