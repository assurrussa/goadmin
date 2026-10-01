<template>
  <div v-if="password" class="space-y-2">
    <div class="text-sm font-medium text-text-primary">Надежность пароля:</div>
    <div class="flex space-x-1">
      <div
        v-for="i in 4"
        :key="i"
        class="h-2 flex-1 rounded-full transition-colors duration-300"
        :class="getStrengthClass(i)"
      />
    </div>
    <div class="text-xs text-text-secondary">
      {{ strengthText }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  password: string
  minLength?: number
}

const props = withDefaults(defineProps<Props>(), {
  minLength: 8,
})

// Оценка надежности пароля
const strength = computed(() => {
  const pwd = props.password
  if (!pwd) return 0

  let score = 0

  // Длина
  if (pwd.length >= props.minLength) score++
  if (pwd.length >= 12) score++

  // Содержит цифры
  if (/\d/.test(pwd)) score++

  // Содержит специальные символы или заглавные буквы
  if (/[A-Z]/.test(pwd) || /[!@#$%^&*(),.?":{}|<>]/.test(pwd)) score++

  return Math.min(score, 4)
})

// Текст для индикатора надежности пароля
const strengthText = computed(() => {
  switch (strength.value) {
    case 0:
    case 1:
      return 'Слабый пароль'
    case 2:
      return 'Средний пароль'
    case 3:
      return 'Хороший пароль'
    case 4:
      return 'Отличный пароль'
    default:
      return ''
  }
})

// Класс для индикатора надежности пароля
const getStrengthClass = (index: number) => {
  if (index <= strength.value) {
    if (strength.value <= 1) return 'bg-error'
    if (strength.value <= 2) return 'bg-warning'
    if (strength.value <= 3) return 'bg-info'
    return 'bg-success'
  }
  return 'bg-surface-variant'
}
</script>
