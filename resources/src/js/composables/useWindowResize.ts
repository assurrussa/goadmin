import { ref, onMounted, onUnmounted } from 'vue'

/**
 * Composable для отслеживания изменения размеров окна
 * @returns объект с текущими размерами окна
 */
export function useWindowResize() {
  // Проверяем, что мы в браузере
  if (typeof window === 'undefined')
    return {
      width: ref(0),
      height: ref(0),
    }

  const width = ref(window.innerWidth)
  const height = ref(window.innerHeight)

  const handleResize = () => {
    width.value = window.innerWidth
    height.value = window.innerHeight
  }

  onMounted(() => {
    window.addEventListener('resize', handleResize)
  })

  onUnmounted(() => {
    window.removeEventListener('resize', handleResize)
  })

  return {
    width,
    height,
  }
}

export default useWindowResize
