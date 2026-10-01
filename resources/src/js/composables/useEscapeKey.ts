import { onMounted, onUnmounted } from 'vue'

/**
 * Composable для обработки нажатия клавиши Escape
 * @param callback - функция, вызываемая при нажатии Escape
 */
export function useEscapeKey(callback: () => void) {
  // Проверяем, что мы в браузере
  if (typeof window === 'undefined') return

  const handleKeyDown = (event: KeyboardEvent) => {
    if (event.key === 'Escape') {
      callback()
    }
  }

  onMounted(() => {
    document.addEventListener('keydown', handleKeyDown)
  })

  onUnmounted(() => {
    document.removeEventListener('keydown', handleKeyDown)
  })

  return handleKeyDown
}

export default useEscapeKey
