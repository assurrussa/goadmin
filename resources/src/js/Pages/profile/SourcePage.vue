<template>
  <AppHead title="Источник профиля" />
  <PageActionBar back-href="/auth/profile" label="Источник профиля" :use-history="false" />
  <Card>
    <CardContent class="space-y-4 pt-6">
      <p>
        Локальные значения сохраняются. Отметьте только поля, которые хотите показывать из AuthHub.
      </p>
      <p>Логин здесь является подписью профиля и не меняет способ входа, привязку или права.</p>
      <p v-if="completionUnavailable" role="alert">Автоматическое завершение недоступно. Профиль не изменён.</p>
      <p v-if="completionReady && !completionUnavailable" role="status">Завершаем подтверждённое обновление. При ошибке автоматического повтора не будет.</p>
      <p v-if="!presentation.version">Сначала явно войдите через связанный SSO-аккаунт.</p>
      <form v-else class="space-y-3" @submit.prevent="save">
        <label class="block"
          ><input v-model="form.fields.name" type="checkbox" /> Имя из AuthHub</label
        >
        <label class="block"
          ><input v-model="form.fields.login" type="checkbox" /> Подпись логина из AuthHub</label
        >
        <label v-if="avatarAvailable" class="block"
          ><input v-model="form.fields.avatar" type="checkbox" /> Аватар из AuthHub</label
        >
        <p v-if="avatarAvailable">
          При показе внешнего аватара браузер обращается к разрешённому серверу изображения. Он
          увидит ваш IP-адрес. Referrer не передаётся.
        </p>
        <p v-else>Внешние аватары отключены. Используется локальный аватар.</p>
        <p v-if="presentation.updatedAt">
          Последнее подтверждённое обновление: {{ presentation.updatedAt }}
        </p>
        <p v-else>
          Подтверждённого снимка пока нет. До обновления показываются локальные значения.
        </p>
        <p>
          Сначала сохраните выбор. Обновление не меняет его. Обновление может попросить войти в SSO
          и затем подтвердить обычный вход в админку. Автоматического обновления и повторных попыток
          нет.
        </p>
        <div class="flex flex-wrap gap-3">
          <AppButton type="submit" :disabled="form.processing">Сохранить выбор</AppButton>
          <AppButton type="button" :disabled="form.processing" @click="refresh"
            >Обновить из SSO</AppButton
          >
          <AppButton type="button" :disabled="form.processing" @click="cancel"
            >Отменить ожидающее обновление</AppButton
          >
        </div>
        <p v-if="form.errors.version">{{ form.errors.version }}</p>
      </form>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { useForm } from '@inertiajs/vue3'
import { onMounted, ref } from 'vue'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Card, CardContent } from '@/components/ui/card'

type Presentation = {
  version: string
  updatedAt: string
  fields: { name: boolean; login: boolean; avatar: boolean }
}
const props = defineProps<{ presentation: Presentation; avatarAvailable: boolean; completionReady?: boolean }>()
const form = useForm({
  version: props.presentation.version,
  fields: { ...props.presentation.fields },
})
const completionAttemptKey = 'goadmin.profile.completion-attempted'
const completionUnavailable = ref(false)
// Consume navigation intent before the request; reload/back never auto-retries.
// No selector, claim or session identifier enters page props or JS.
onMounted(() => {
  if (!props.completionReady) return
  try {
    window.history.replaceState(window.history.state, '', '/profile-source')
    if (window.sessionStorage.getItem(completionAttemptKey)) return
    window.sessionStorage.setItem(completionAttemptKey, '1')
  } catch {
    completionUnavailable.value = true
    return
  }
  form.post('/profile-source/complete', { preserveState: 'errors' })
})
function save() {
  form.post('/profile-source', { preserveState: 'errors' })
}
function refresh() {
  try { window.sessionStorage.removeItem(completionAttemptKey) } catch { /* Completion will fail closed. */ }
  form.post('/profile-source/refresh', { preserveState: 'errors' })
}
function cancel() {
  form.post('/profile-source/cancel', { preserveState: 'errors' })
}
</script>
