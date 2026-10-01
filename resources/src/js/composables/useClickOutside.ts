import { ref, onMounted, onUnmounted } from 'vue'

/**
 * Composable для обработки кликов вне элемента
 * @param targetRef - реф на элемент, вне которого отслеживаются клики
 * @param callback - функция, вызываемая при клике вне элемента
 */
export function useClickOutside(
  targetRef: ReturnType<typeof ref<HTMLElement | null>>,
  callback: () => void,
) {
  // Проверяем, что мы в браузере
  if (typeof window === 'undefined') return

  const handleClick = (event: MouseEvent) => {
    if (!targetRef.value) return

    if (!targetRef.value.contains(event.target as Node)) {
      callback()
    }
  }

  onMounted(() => {
    document.addEventListener('click', handleClick)
  })

  onUnmounted(() => {
    document.removeEventListener('click', handleClick)
  })

  return handleClick
}

export default useClickOutside
