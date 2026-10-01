import { ref, onMounted, watch } from 'vue'

// Переменные для глобального состояния скролла
const globalScrollY = ref(0)
const globalScrollDirection = ref('none')
const globalLastScrollY = ref(0)

// Глобальный обработчик события скролла
const handleGlobalScroll = () => {
  // Безопасная проверка для SSR
  if (typeof window === 'undefined') return

  const currentScrollY = window.scrollY

  // Определяем направление скролла
  if (currentScrollY > globalLastScrollY.value) {
    globalScrollDirection.value = 'down'
  } else if (currentScrollY < globalLastScrollY.value) {
    globalScrollDirection.value = 'up'
  }

  // Обновляем значение текущей позиции скролла
  globalScrollY.value = currentScrollY
  globalLastScrollY.value = currentScrollY
}

// Устанавливаем глобальный обработчик событий скролла только один раз
if (typeof window !== 'undefined') {
  // Инициализируем значения
  globalScrollY.value = window.scrollY
  // Добавляем обработчик события
  window.addEventListener('scroll', handleGlobalScroll, { passive: true })
}

export function useScroll() {
  // Порог, после которого считаем, что страница прокручена достаточно для показа кнопки
  const scrollThreshold = ref(300)

  // Флаг для проверки, прокручена ли страница выше порога
  const isScrolled = ref(false)

  // Локальная проверка прокрутки, которая использует глобальное значение scrollY
  const checkIfScrolled = () => {
    isScrolled.value = globalScrollY.value > scrollThreshold.value
  }

  // Функция для плавной прокрутки страницы наверх
  const scrollToTop = () => {
    if (typeof window === 'undefined') return

    window.scrollTo({
      top: 0,
      behavior: 'smooth',
    })
  }

  // Добавляем и обновляем проверку при монтировании и изменении порога
  onMounted(() => {
    // Инициализация значений при монтировании
    checkIfScrolled()

    // Создаем наблюдатель за изменением глобального scrollY
    watch(globalScrollY, () => {
      checkIfScrolled()
    })
  })

  // Устанавливаем обработчик изменения значения scrollThreshold
  const updateScrollThreshold = (newThreshold: number) => {
    scrollThreshold.value = newThreshold
    checkIfScrolled()
  }

  return {
    scrollY: globalScrollY,
    scrollDirection: globalScrollDirection,
    isScrolled,
    scrollThreshold,
    scrollToTop,
    updateScrollThreshold,
  }
}
