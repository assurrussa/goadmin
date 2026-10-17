<template>
  <AuthLayout>
    <template #title>{{ t('Настройте двухфакторную защиту') }}</template>
    <template #subtitle>{{
      t('Для входа в эту панель нужен код из приложения-аутентификатора.')
    }}</template>

    <p v-if="error" role="alert" class="text-sm text-error">{{ error }}</p>
    <form v-if="enrollment" class="space-y-4" @submit.prevent="confirm">
      <p>
        {{ t('Добавьте этот ключ в приложение-аутентификатор, затем введите полученный код.') }}
      </p>
      <p class="font-mono break-all rounded border p-3" :aria-label="t('Ключ настройки')">
        {{ enrollment.secret }}
      </p>
      <p>
        {{
          t(
            'Сохраните резервные коды в безопасном месте. Они станут действовать только после подтверждения настройки.',
          )
        }}
      </p>
      <ul class="font-mono text-sm space-y-1" :aria-label="t('Резервные коды')">
        <li v-for="recoveryCode in enrollment.recoveryCodes" :key="recoveryCode">
          {{ recoveryCode }}
        </li>
      </ul>
      <label class="flex items-center gap-2 text-sm">
        <input v-model="codesSaved" type="checkbox" />
        {{ t('Резервные коды сохранены') }}
      </label>
      <FormInput
        v-model="code"
        name="code"
        :label="t('Код подтверждения')"
        autocomplete="one-time-code"
        required
      />
      <FormButton
        type="submit"
        :loading="busy"
        :disabled="busy || !code || !codesSaved"
        full-width
        >{{ t('Подтвердить и войти') }}</FormButton
      >
    </form>
    <div v-else class="space-y-4">
      <p>
        {{
          t(
            'До подтверждения настройки доступ к панели закрыт. Можно вернуться ко входу и продолжить позже.',
          )
        }}
      </p>
      <FormButton
        type="button"
        :loading="busy"
        :disabled="busy || leaving"
        full-width
        @click="begin"
        >{{ t('Начать настройку') }}</FormButton
      >
    </div>

    <template #footer>
      <Link href="/auth/login" @click="leave">{{ t('Вернуться ко входу') }}</Link>
    </template>
  </AuthLayout>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { onBeforeUnmount, ref } from 'vue'
import { Link } from '@inertiajs/vue3'
import axios from 'axios'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import { bindEphemeralAuthPage } from '@/lib/ephemeral-auth'

const props = defineProps<{ challengeID: string }>()
type Enrollment = {
  id: string
  secret: string
  url: string
  expiresAt: string
  recoveryCodes: string[]
}
const enrollment = ref<Enrollment | null>(null)
const codesSaved = ref(false)
const code = ref('')
const error = ref('')
const busy = ref(false)
const leaving = ref(false)
const lifecycle = bindEphemeralAuthPage(clearSecrets)
onBeforeUnmount(lifecycle.dispose)

function isActive() {
  return lifecycle.isActive() && !leaving.value
}

function leave() {
  leaving.value = true
  error.value = ''
  clearSecrets()
}

function clearSecrets() {
  enrollment.value = null
  codesSaved.value = false
  code.value = ''
}

function validEnrollment(value: unknown): value is Enrollment {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<Enrollment>
  return (
    typeof item.id === 'string' &&
    item.id.length > 0 &&
    typeof item.secret === 'string' &&
    item.secret.length > 0 &&
    typeof item.url === 'string' &&
    typeof item.expiresAt === 'string' &&
    Array.isArray(item.recoveryCodes) &&
    item.recoveryCodes.length > 0 &&
    item.recoveryCodes.every((value) => typeof value === 'string' && value.length > 0)
  )
}

async function begin() {
  if (!isActive() || busy.value) return
  busy.value = true
  error.value = ''
  clearSecrets()
  try {
    const response = await axios.post('/auth/mfa/enrollment/begin', {
      challengeID: props.challengeID,
    })
    if (!isActive()) return
    if (!validEnrollment(response.data?.enrollment)) throw new Error('Invalid setup response')
    enrollment.value = response.data.enrollment
  } catch {
    if (isActive())
      error.value = t('Не удалось начать настройку. Вернитесь ко входу и попробуйте снова.')
  } finally {
    if (isActive()) busy.value = false
  }
}

async function confirm() {
  if (!isActive() || busy.value || !enrollment.value || !code.value || !codesSaved.value) return
  busy.value = true
  error.value = ''
  try {
    const response = await axios.post('/auth/mfa/enrollment/confirm', {
      challengeID: props.challengeID,
      enrollmentID: enrollment.value.id,
      code: code.value,
    })
    if (!isActive()) return
    const result = response.data
    if (
      typeof result?.redirect !== 'string' ||
      !result.redirect.startsWith('/') ||
      result.redirect.startsWith('//') ||
      result.redirect.includes('\\')
    ) {
      throw new Error('Invalid setup confirmation')
    }
    const target = new URL(result.redirect, window.location.origin)
    if (target.origin !== window.location.origin) {
      throw new Error('Invalid setup confirmation')
    }
    clearSecrets()
    window.location.assign(target.href)
  } catch {
    if (isActive())
      error.value = t('Не удалось подтвердить настройку. Проверьте код или начните вход заново.')
  } finally {
    code.value = ''
    if (isActive()) busy.value = false
  }
}
</script>
