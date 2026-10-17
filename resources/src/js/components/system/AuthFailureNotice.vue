<script setup lang="ts">
import { t } from '@/i18n'
import { computed } from 'vue'

const props = defineProps<{ message: string; loginRequired?: boolean }>()
const notice = computed(() => {
  const match = props.message.match(
    /^Сервис авторизации временно недоступен\. Данные формы сохранены на странице\. Повторите отправку вручную через (несколько секунд|\d+ с)\.$/,
  )
  if (!match) return t(props.message)
  const delay =
    match[1] === 'несколько секунд'
      ? t('несколько секунд')
      : t('{seconds} с', undefined, { seconds: match[1]!.split(' ')[0]! })
  return t(
    'Сервис авторизации временно недоступен. Данные формы сохранены на странице. Повторите отправку вручную через {delay}.',
    undefined,
    { delay },
  )
})
defineEmits<{ dismiss: []; login: [] }>()
</script>

<template>
  <aside
    v-if="message"
    role="alert"
    aria-live="assertive"
    aria-atomic="true"
    class="fixed bottom-4 left-4 right-4 z-50 mx-auto max-w-xl rounded-lg border border-border-primary bg-card p-4 text-text-primary shadow-lg"
  >
    <p class="pr-6">{{ notice }}</p>
    <button
      v-if="loginRequired"
      type="button"
      class="mr-3 mt-3 rounded border border-border-primary px-3 py-1 focus-visible:outline"
      @click="$emit('login')"
    >
      {{ t('Войти снова') }}
    </button>
    <button
      type="button"
      class="mt-3 rounded border border-border-primary px-3 py-1 focus-visible:outline"
      @click="$emit('dismiss')"
    >
      {{ t('Закрыть уведомление') }}
    </button>
  </aside>
</template>
