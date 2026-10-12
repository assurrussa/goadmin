<template>
  <section class="rounded-xl border border-border-secondary p-6 space-y-4">
    <h2>Двухфакторная защита</h2>
    <p>{{ enabled ? 'Защита включена' : 'Защита не включена' }}</p>
    <p v-if="message" role="status">{{ message }}</p>
    <template v-if="!signedOut">
      <PasswordInput
        v-model="password"
        name="mfaPassword"
        label="Текущий пароль"
        autocomplete="current-password"
      />
      <FormInput
        v-model="code"
        name="mfaCode"
        label="Код приложения или резервный код"
        autocomplete="one-time-code"
      />
      <template v-if="enrollment">
        <p>Добавьте секрет в приложение-аутентификатор. Он показан только сейчас.</p>
        <p class="break-all select-all">{{ enrollment.secret }}</p>
        <FormButton :disabled="busy" @click="run('confirm')">Подтвердить новый код</FormButton>
        <FormButton :disabled="busy" @click="run('cancel')">Отмена</FormButton>
      </template>
      <template v-else>
        <FormButton :disabled="busy" @click="run('begin')">{{
          enabled ? 'Заменить приложение' : 'Включить защиту'
        }}</FormButton>
        <FormButton v-if="enabled" :disabled="busy" @click="run('recovery')"
          >Новые резервные коды</FormButton
        >
        <FormButton v-if="enabled" :disabled="busy" @click="run('disable')"
          >Отключить защиту</FormButton
        >
      </template>
    </template>
    <template v-if="recoveryCodes.length">
      <p>
        Сохраните резервные коды. Каждый действует один раз. После ухода со страницы они больше не
        показываются.
      </p>
      <ul>
        <li v-for="item in recoveryCodes" :key="item" class="select-all">{{ item }}</li>
      </ul>
    </template>
    <a v-if="signedOut" href="/auth/login">Войти заново</a>
  </section>
</template>
<script setup lang="ts">
import { bindEphemeralAuthPage } from '@/lib/ephemeral-auth'
import axios from 'axios'
import { onBeforeUnmount, ref } from 'vue'
import PasswordInput from '@/components/form/PasswordInput.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
defineProps<{ enabled: boolean }>()
const password = ref('')
const code = ref('')
const busy = ref(false)
const message = ref('')
const signedOut = ref(false)
const enrollment = ref<{ id: string; secret: string } | null>(null)
const recoveryCodes = ref<string[]>([])
function clearSecrets() {
  password.value = ''
  code.value = ''
  enrollment.value = null
  recoveryCodes.value = []
}
const lifecycle = bindEphemeralAuthPage(clearSecrets)
onBeforeUnmount(lifecycle.dispose)
async function run(action: string) {
  if (!lifecycle.isActive() || busy.value) return
  busy.value = true
  message.value = ''
  try {
    const { data } = await axios.post('/auth/profile/settings/mfa/' + action, {
      password: password.value,
      code: code.value,
      enrollmentID: enrollment.value?.id || '',
    })
    if (!lifecycle.isActive()) return
    if (
      !data ||
      typeof data !== 'object' ||
      typeof data.signedOut !== 'boolean' ||
      (action === 'begin' &&
        (!data.enrollment ||
          typeof data.enrollment.id !== 'string' ||
          typeof data.enrollment.secret !== 'string')) ||
      ((action === 'confirm' || action === 'recovery') &&
        (!Array.isArray(data.recoveryCodes) ||
          !data.recoveryCodes.length ||
          !data.recoveryCodes.every((item: unknown) => typeof item === 'string'))) ||
      ((action === 'confirm' || action === 'recovery' || action === 'disable') && !data.signedOut)
    )
      throw new Error('Invalid MFA response')
    enrollment.value = data.enrollment || null
    recoveryCodes.value = data.recoveryCodes || []
    signedOut.value = Boolean(data.signedOut)
    message.value = signedOut.value
      ? 'Настройки сохранены. Все сеансы завершены.'
      : action === 'cancel'
        ? 'Настройка отменена.'
        : 'Введите код нового приложения для подтверждения.'
  } catch {
    if (lifecycle.isActive())
      message.value =
        'Не удалось выполнить операцию. Проверьте пароль и код. После потери ответа войдите заново и проверьте состояние защиты.'
  } finally {
    password.value = ''
    code.value = ''
    busy.value = false
  }
}
</script>
