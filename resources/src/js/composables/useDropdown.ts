import { ref } from 'vue'
import useClickOutside from './useClickOutside'
import useEscapeKey from './useEscapeKey'

/**
 * Composable для управления выпадающим меню или модальным окном
 * @param options - опции для настройки поведения
 * @returns объект с состоянием и методами для управления выпадающим элементом
 */
export function useDropdown(
  options: {
    closeOnEscape?: boolean
    closeOnClickOutside?: boolean
    initialState?: boolean
    onOpen?: () => void
    onClose?: () => void
  } = {},
) {
  const {
    closeOnEscape = true,
    closeOnClickOutside = true,
    initialState = false,
    onOpen,
    onClose,
  } = options

  const isOpen = ref(initialState)
  const dropdownRef = ref<HTMLElement | null>(null)

  // Методы управления состоянием
  const open = () => {
    isOpen.value = true
    onOpen?.()
  }

  const close = () => {
    isOpen.value = false
    onClose?.()
  }

  const toggle = () => {
    if (isOpen.value) {
      close()
    } else {
      open()
    }
  }

  // Настраиваем обработчики событий
  const clickOutsideHandler = closeOnClickOutside
    ? useClickOutside(dropdownRef, () => {
        if (isOpen.value) {
          close()
        }
      })
    : null

  const escapeKeyHandler = closeOnEscape
    ? useEscapeKey(() => {
        if (isOpen.value) {
          close()
        }
      })
    : null

  // Предотвращаем tree-shaking обработчиков
  void clickOutsideHandler
  void escapeKeyHandler

  return {
    isOpen,
    dropdownRef,
    open,
    close,
    toggle,
  }
}

export default useDropdown
