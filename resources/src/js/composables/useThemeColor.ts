import { ref } from 'vue'

const presets = {
  // Основные цвет проекта
  theme: { hue: 220, saturation: 55 },

  // Основные цвета
  blue: { hue: 220, saturation: 90 },
  red: { hue: 0, saturation: 85 },
  green: { hue: 142, saturation: 70 },
  purple: { hue: 270, saturation: 80 },
  orange: { hue: 30, saturation: 90 },
  teal: { hue: 180, saturation: 70 },

  // Пастельные и нежные тона
  skyblue: { hue: 195, saturation: 55 }, // Нежно-голубой
  lavender: { hue: 250, saturation: 40 }, // Лавандовый/сиреневый
  mint: { hue: 160, saturation: 30 }, // Мятный
  peach: { hue: 25, saturation: 50 }, // Персиковый
  rose: { hue: 350, saturation: 40 }, // Розовый
  lilac: { hue: 280, saturation: 35 }, // Сиренево-лиловый
} as const

/**
 * Composable для управления цветовой схемой приложения
 * Позволяет динамически менять основной цвет сайта
 */
export function useThemeColor() {
  // Текущие значения HSL для основного цвета
  const primaryHue = ref(220) // По умолчанию синий
  const primarySaturation = ref(90)

  /**
   * Изменяет основной цвет сайта
   * @param {number} hue - Цветовой тон (0-360)
   * @param {number} saturation - Насыщенность (0-100)
   */
  function setThemeColor(hue: number, saturation: number) {
    if (hue >= 0 && hue <= 360) {
      primaryHue.value = hue
      document.documentElement.style.setProperty('--hue-primary', hue.toString())
    }

    if (saturation >= 0 && saturation <= 100) {
      primarySaturation.value = saturation
      document.documentElement.style.setProperty('--saturation-primary', `${saturation}%`)
    }
  }

  /**
   * Изменяет основной цвет сайта на предустановленный
   * @param {string} colorName - Название предустановленного цвета
   */
  function setPresetColor(colorName: keyof typeof presets) {
    if (presets[colorName]) {
      const { hue, saturation } = presets[colorName]
      setThemeColor(hue, saturation)
    }
  }

  /**
   * Возвращает HSL строку для текущего основного цвета
   * @param {number} lightness - Яркость (0-100)
   * @returns {string} HSL строка
   */
  function getThemeColorHsl(lightness = 50): string {
    return `hsl(${primaryHue.value}, ${primarySaturation.value}%, ${lightness}%)`
  }

  // При инициализации берем значения из CSS переменных, если они заданы
  function init() {
    const rootStyles = getComputedStyle(document.documentElement)
    const hueFromCSS = rootStyles.getPropertyValue('--hue-primary').trim()
    const satFromCSS = rootStyles.getPropertyValue('--saturation-primary').trim()

    if (hueFromCSS) {
      primaryHue.value = parseInt(hueFromCSS, 10)
    }

    if (satFromCSS) {
      primarySaturation.value = parseInt(satFromCSS, 10)
    }
  }

  // Инициализируем при создании только на клиенте
  if (typeof window !== 'undefined') {
    init()
  }

  return {
    primaryHue,
    primarySaturation,
    setThemeColor,
    setPresetColor,
    getThemeColorHsl,
  }
}
