/** Reka disabled prevents selection, while consumer listeners and link defaults
 * still need fencing when controls remain hit-testable for their cursor. */
export function disabledActivation(disabled: boolean | undefined) {
  if (!disabled) return {}
  const suppress = (event: Event) => {
    if (event instanceof KeyboardEvent && !['Enter', ' '].includes(event.key)) return
    event.preventDefault()
    event.stopImmediatePropagation()
  }
  return {
    onClickCapture: suppress,
    onKeydownCapture: suppress,
    onKeyupCapture: suppress,
    onPointerdownCapture: suppress,
    onPointerupCapture: suppress,
  }
}
