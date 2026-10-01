import { computed, type Ref } from 'vue'

export interface PasswordValidationOptions {
  minLength?: number
  requireNumbers?: boolean
  requireUppercase?: boolean
  requireSpecialChars?: boolean
}

export interface PasswordStrength {
  score: number
  text: string
  class: string
}

export function usePasswordValidation(
  password: Ref<string>,
  confirmPassword?: Ref<string>,
  options: PasswordValidationOptions = {},
) {
  const {
    minLength = 8,
    requireNumbers = false,
    requireUppercase = false,
    requireSpecialChars = false,
  } = options

  // Оценка надежности пароля
  const passwordStrength = computed((): PasswordStrength => {
    const pwd = password.value
    if (!pwd) return { score: 0, text: '', class: 'bg-surface-variant' }

    let score = 0

    // Длина
    if (pwd.length >= minLength) score++
    if (pwd.length >= 12) score++

    // Содержит цифры
    if (/\d/.test(pwd)) score++

    // Содержит специальные символы или заглавные буквы
    if (/[A-Z]/.test(pwd) || /[!@#$%^&*(),.?":{}|<>]/.test(pwd)) score++

    const finalScore = Math.min(score, 4)

    let text = ''
    let cssClass = ''

    switch (finalScore) {
      case 0:
      case 1:
        text = 'Слабый пароль'
        cssClass = 'bg-error'
        break
      case 2:
        text = 'Средний пароль'
        cssClass = 'bg-warning'
        break
      case 3:
        text = 'Хороший пароль'
        cssClass = 'bg-info'
        break
      case 4:
        text = 'Отличный пароль'
        cssClass = 'bg-success'
        break
    }

    return { score: finalScore, text, class: cssClass }
  })

  // Валидация основного пароля
  const passwordErrors = computed(() => {
    const pwd = password.value
    const errors: string[] = []

    if (!pwd) return errors

    if (pwd.length < minLength) {
      errors.push(`Пароль должен содержать минимум ${minLength} символов`)
    }

    if (requireNumbers && !/\d/.test(pwd)) {
      errors.push('Пароль должен содержать цифры')
    }

    if (requireUppercase && !/[A-Z]/.test(pwd)) {
      errors.push('Пароль должен содержать заглавные буквы')
    }

    if (requireSpecialChars && !/[!@#$%^&*(),.?":{}|<>]/.test(pwd)) {
      errors.push('Пароль должен содержать специальные символы')
    }

    return errors
  })

  // Валидация подтверждения пароля
  const confirmPasswordError = computed(() => {
    if (!confirmPassword) return null

    const confirm = confirmPassword.value
    const pwd = password.value

    if (confirm && pwd !== confirm) {
      return 'Пароли не совпадают'
    }

    return null
  })

  // Проверка валидности формы
  const isPasswordValid = computed(() => {
    return passwordErrors.value.length === 0 && password.value.length >= minLength
  })

  const isFormValid = computed(() => {
    if (!confirmPassword) {
      return isPasswordValid.value
    }

    return (
      isPasswordValid.value &&
      confirmPasswordError.value === null &&
      password.value === confirmPassword.value
    )
  })

  // Класс для индикатора надежности пароля
  const getPasswordStrengthClass = (index: number) => {
    if (index <= passwordStrength.value.score) {
      return passwordStrength.value.class
    }
    return 'bg-surface-variant'
  }

  return {
    passwordStrength,
    passwordErrors,
    confirmPasswordError,
    isPasswordValid,
    isFormValid,
    getPasswordStrengthClass,
  }
}
