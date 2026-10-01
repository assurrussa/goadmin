/**
 * Плагин для инициализации системы тем в админке
 * - Синхронизируется с темой, примененной через inline script
 * - Загружает сохраненную тему из localStorage
 * - Применяет тему к документу
 */

// Инициализация темы (выполняется только на клиенте)
function initTheme() {
  if (typeof window !== 'undefined') {
    // Получить сохраненные настройки или использовать предпочтения системы
    const savedTheme = localStorage.getItem('theme-mode')
    const systemDarkMode =
      window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches
    const shouldBeDark = savedTheme === 'dark' || (savedTheme === null && systemDarkMode)

    // Проверяем текущее состояние DOM - только меняем если нужно
    const isDarkNow = document.documentElement.classList.contains('dark')

    // Применить тему только если текущее состояние не соответствует ожидаемому
    if (shouldBeDark && !isDarkNow) {
      document.documentElement.classList.add('dark')
      document.documentElement.setAttribute('data-theme-mode', 'dark')
    } else if (!shouldBeDark && isDarkNow) {
      document.documentElement.classList.remove('dark')
      document.documentElement.setAttribute('data-theme-mode', 'light')
    } else {
      // Убедимся, что атрибут data-theme-mode соответствует текущему классу
      document.documentElement.setAttribute('data-theme-mode', isDarkNow ? 'dark' : 'light')
    }

    // Добавить слушатель для системных настроек
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
      if (localStorage.getItem('theme-mode') === null) {
        if (e.matches) {
          document.documentElement.classList.add('dark')
          document.documentElement.setAttribute('data-theme-mode', 'dark')
        } else {
          document.documentElement.classList.remove('dark')
          document.documentElement.setAttribute('data-theme-mode', 'light')
        }
      }
    })
  }
}

// Получить текущую тему
function getCurrentTheme() {
  if (typeof window !== 'undefined') {
    const isDark =
      document.documentElement.classList.contains('dark') ||
      document.documentElement.getAttribute('data-theme-mode') === 'dark'
    return isDark ? 'dark' : 'light'
  }
  return 'light'
}

// Переключить между светлой и темной темой
function toggleDarkMode() {
  if (typeof window !== 'undefined') {
    const isDark = document.documentElement.classList.contains('dark')
    if (isDark) {
      setLightMode()
    } else {
      setDarkMode()
    }
  }
}

// Установить темную тему
function setDarkMode() {
  if (typeof window !== 'undefined') {
    document.documentElement.classList.add('dark')
    document.documentElement.setAttribute('data-theme-mode', 'dark')
    localStorage.setItem('theme-mode', 'dark')
  }
}

// Установить светлую тему
function setLightMode() {
  if (typeof window !== 'undefined') {
    document.documentElement.classList.remove('dark')
    document.documentElement.setAttribute('data-theme-mode', 'light')
    localStorage.setItem('theme-mode', 'light')
  }
}

// Экспортируем функции для использования в других частях приложения
export const themeManager = {
  init: initTheme,
  toggle: toggleDarkMode,
  setDark: setDarkMode,
  setLight: setLightMode,
  getCurrent: getCurrentTheme,
}

// Инициализируем тему при загрузке плагина
if (typeof window !== 'undefined') {
  initTheme()
}
