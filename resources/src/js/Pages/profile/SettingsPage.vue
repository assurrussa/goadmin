<template>
  <AppHead :title="props.title" />
  <PageActionBar
    back-href="/auth/profile"
    :back-text="t('К профилю')"
    :use-history="false"
    :label="t('Настройки')"
  />

  <h1 class="mb-6 text-2xl font-semibold text-text-primary">{{ t('Настройки') }}</h1>
  <nav
    :aria-label="t('Разделы настроек')"
    class="mb-6 flex flex-wrap gap-2 border-b border-border-primary pb-3"
  >
    <a
      v-for="item in settingsSections"
      :key="item.key"
      :href="'#' + item.key"
      :aria-current="section === item.key ? 'page' : undefined"
      class="inline-flex min-h-11 items-center rounded-lg px-4 text-sm font-medium hover:bg-surface-variant"
      :class="section === item.key ? 'bg-primary/10 text-primary' : 'text-text-secondary'"
      @click="section = item.key"
      >{{ t(item.label) }}</a
    >
  </nav>
  <div class="space-y-8">
    <AppearanceSettings v-if="section === 'interface'" />
    <BrandingSettings v-if="section === 'branding' && appearance.brandingRead" />
    <p v-if="section === 'account' && !authmail && !ssoEnabled" class="text-text-secondary">
      {{ t('Дополнительные настройки учётной записи недоступны.') }}
    </p>
    <section
      v-if="ssoEnabled"
      v-show="section === 'account'"
      aria-labelledby="sso-settings-title"
      class="space-y-4"
    >
      <h2 id="sso-settings-title" class="text-xl font-semibold">AuthHub SSO</h2>
      <Card>
        <CardHeader class="border-b border-border-secondary bg-surface-variant/40">
          <CardTitle>Связь учётной записи</CardTitle>
          <CardDescription
            >Вход через SSO использует ваши существующие локальные права.</CardDescription
          >
        </CardHeader>
        <CardContent class="space-y-3 pt-6">
          <p v-if="ssoLoading" role="status">Проверяем состояние привязки…</p>
          <p v-else-if="ssoError" role="alert">
            Состояние привязки временно недоступно. Обновите страницу позже.
          </p>
          <template v-else-if="ssoAccount">
            <p v-if="ssoAccount.linked" role="status">Учётная запись AuthHub привязана.</p>
            <template v-else>
              <p role="status">Учётная запись AuthHub не привязана.</p>
              <p v-if="ssoAccount.pending" class="text-sm text-text-secondary">
                Вход через AuthHub подтверждён. Завершите привязку, чтобы использовать SSO в этой
                админке.
              </p>
              <p v-if="ssoAccount.localAuth === 'stale'" class="text-sm text-text-secondary">
                Для привязки потребуется новый локальный вход.
              </p>
              <p v-if="ssoAccount.localAuth === 'not_local'" class="text-sm text-text-secondary">
                Для привязки завершите текущий сеанс через меню профиля и войдите локально. Выход из
                AuthHub не выполняется автоматически.
              </p>
              <a
                v-if="ssoAccount.localAuth !== 'not_local'"
                :href="
                  ssoAccount.pending
                    ? '/sso-link'
                    : '/sso?redirectback=%2Fauth%2Fprofile%2Fsettings'
                "
                class="text-primary"
              >
                {{ ssoAccount.pending ? 'Завершить привязку' : 'Привязать AuthHub' }}
              </a>
            </template>
          </template>
        </CardContent>
      </Card>
    </section>
    <section
      v-if="authmail"
      v-show="section === 'account'"
      aria-labelledby="profile-settings-title"
      class="space-y-4"
    >
      <h2 id="profile-settings-title" class="text-xl font-semibold">{{ t('Профиль') }}</h2>
      <Card>
        <CardHeader class="border-b border-border-secondary bg-surface-variant/40">
          <CardTitle>{{ t('Смена email') }}</CardTitle>
          <CardDescription>
            {{
              t('Для смены email мы отправим код подтверждения на новый адрес и уведомим старый.')
            }}
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-6 pt-6">
          <form
            :aria-label="t('Запрос смены email')"
            class="space-y-6"
            @submit.prevent="sendEmailCode"
          >
            <div class="flex flex-col gap-2 text-sm">
              <span class="text-text-tertiary">{{ t('Текущий email') }}</span>
              <span class="text-text-primary font-medium">{{ props.data.email }}</span>
            </div>

            <PasswordInput
              v-model="emailRequestForm.currentPassword"
              name="currentPassword"
              :label="t('Текущий пароль')"
              :placeholder="t('Введите текущий пароль')"
              autocomplete="current-password"
              :error="emailRequestForm.errors.currentPassword"
            />

            <div class="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-end">
              <FormInput
                v-model="emailRequestForm.email"
                name="email"
                type="email"
                :label="t('Новый email')"
                placeholder="name@example.com"
                :error="emailRequestForm.errors.email"
              />
              <FormButton
                type="submit"
                variant="secondary"
                :loading="emailRequestForm.processing"
                :loading-text="t('Отправляем...')"
                class="lg:mb-1"
                :disabled="emailRequestForm.processing || emailConfirmForm.processing"
              >
                {{ t('Отправить код') }}
              </FormButton>
            </div>
          </form>
          <div
            v-if="pendingEmailChange"
            class="rounded-xl border border-border-secondary bg-surface px-4 py-3 text-sm text-text-secondary"
          >
            <p>
              {{ t('Код отправлен на') }}
              <span class="font-medium text-text-primary">{{ pendingEmailChange.newEmail }}</span
              >{{ t('. Действует до') }} {{ formatDateTime(pendingEmailChange.expiresAt) }}.
            </p>
            <p v-if="emailCodeExpired" role="status" class="mt-2">
              {{ t('Срок действия кода истёк. Запросите новый код через форму выше.') }}
            </p>
          </div>

          <form
            v-if="pendingEmailChange && !emailCodeExpired"
            :aria-label="t('Подтверждение email')"
            class="grid gap-4 lg:grid-cols-[1fr_auto] lg:items-end"
            @submit.prevent="confirmEmail"
          >
            <FormInput
              v-model="emailConfirmForm.code"
              name="code"
              autocomplete="one-time-code"
              inputmode="numeric"
              :label="t('Код подтверждения')"
              placeholder="123456"
              :error="emailConfirmForm.errors.code"
              help-text="Введите 6-значный код из письма"
            />
            <FormButton
              type="submit"
              variant="primary"
              :loading="emailConfirmForm.processing"
              :loading-text="t('Проверяем...')"
              class="lg:mb-1"
              :disabled="emailConfirmForm.processing || emailRequestForm.processing"
            >
              {{ t('Подтвердить email') }}
            </FormButton>
          </form>
        </CardContent>
      </Card>
    </section>

    <section
      v-show="section === 'security'"
      aria-labelledby="security-settings-title"
      class="space-y-4"
    >
      <h2 id="security-settings-title" class="text-xl font-semibold">{{ t('Безопасность') }}</h2>
      <LocalMFASettings
        :state="
          props.mfaState ||
          (props.mfaAvailable ? (props.mfaEnabled ? 'enabled' : 'disabled') : 'unavailable')
        "
      />
      <Card>
        <CardHeader class="border-b border-border-secondary bg-surface-variant/40">
          <CardTitle>{{ t('Смена пароля') }}</CardTitle>
          <CardDescription>
            {{
              t(
                'Укажите текущий пароль и задайте новый. После смены пароля все сеансы завершатся, и потребуется войти заново.',
              )
            }}
          </CardDescription>
        </CardHeader>
        <CardContent class="pt-6">
          <form :aria-label="t('Смена пароля')" class="space-y-6" @submit.prevent="changePassword">
            <PasswordInput
              v-model="passwordForm.currentPassword"
              name="currentPassword"
              :label="t('Текущий пароль')"
              :placeholder="t('Введите текущий пароль')"
              autocomplete="current-password"
              :error="passwordForm.errors.currentPassword"
            />
            <PasswordInput
              v-model="passwordForm.password"
              name="password"
              :label="t('Новый пароль')"
              :placeholder="t('Введите новый пароль')"
              autocomplete="new-password"
              :error="passwordForm.errors.password"
            />
            <PasswordInput
              v-model="passwordForm.passwordConfirmation"
              name="passwordConfirmation"
              :label="t('Подтвердите новый пароль')"
              :placeholder="t('Повторите новый пароль')"
              autocomplete="new-password"
              :error="
                passwordForm.errors.passwordConfirmation || passwordConfirmationError || undefined
              "
            />

            <PasswordStrengthIndicator :password="passwordForm.password" />

            <div class="flex justify-end">
              <FormButton
                type="submit"
                variant="primary"
                :loading="passwordForm.processing"
                :loading-text="t('Сохраняем...')"
                :disabled="!isPasswordFormValid || passwordForm.processing"
              >
                {{ t('Обновить пароль') }}
              </FormButton>
            </div>
          </form>
        </CardContent>
      </Card>
    </section>
  </div>
</template>

<script setup lang="ts">
import AppearanceSettings from '@/components/profile/AppearanceSettings.vue'
import BrandingSettings from '@/components/profile/BrandingSettings.vue'
import { appearance } from '@/appearance/state'
import { adminFetch } from '@/http'
import { t, locale } from '@/i18n'

import LocalMFASettings from '@/components/profile/LocalMFASettings.vue'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import PasswordInput from '@/components/form/PasswordInput.vue'
import PasswordStrengthIndicator from '@/components/form/PasswordStrengthIndicator.vue'
import { Card, CardHeader, CardContent, CardTitle, CardDescription } from '@/components/ui/card'

interface PendingEmailChange {
  newEmail: string
  expiresAt: string
}

interface SettingsProps {
  mfaState?: 'unavailable' | 'unknown' | 'enabled' | 'disabled'
  mfaAvailable?: boolean
  mfaEnabled?: boolean
  title?: string
  data: {
    email: string
  }
  pendingEmailChange?: PendingEmailChange | null
}

const props = defineProps<SettingsProps>()
const settingsSections = computed(() => [
  { key: 'interface', label: 'Интерфейс' },
  { key: 'account', label: 'Учётная запись' },
  { key: 'security', label: 'Безопасность' },
  ...(appearance.brandingRead ? [{ key: 'branding', label: 'Бренд админки' }] : []),
])
const section = ref('interface')
const syncSection = () => {
  const hash = window.location.hash.slice(1)
  section.value = settingsSections.value.some((item) => item.key === hash) ? hash : 'interface'
}
onMounted(() => {
  syncSection()
  window.addEventListener('hashchange', syncSection)
})
onUnmounted(() => window.removeEventListener('hashchange', syncSection))
watch(settingsSections, () => {
  if (!settingsSections.value.some((item) => item.key === section.value))
    section.value = 'interface'
})

const { authmail } = useAdminCapabilities()
const page = usePage()
const ssoEnabled = computed(() => page.props.adminSSOEnabled === true)
interface SSOAccount {
  linked: boolean
  pending: boolean
  localAuth: 'fresh' | 'stale' | 'not_local'
}
const ssoAccount = ref<SSOAccount | null>(null)
const ssoLoading = ref(false)
const ssoError = ref(false)
onMounted(async () => {
  if (!ssoEnabled.value) return
  ssoLoading.value = true
  try {
    const response = await adminFetch('/sso-account', {
      credentials: 'same-origin',
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) throw new Error('unavailable')
    const data: unknown = await response.json()
    if (
      !data ||
      typeof data !== 'object' ||
      typeof (data as SSOAccount).linked !== 'boolean' ||
      typeof (data as SSOAccount).pending !== 'boolean' ||
      !['fresh', 'stale', 'not_local'].includes((data as SSOAccount).localAuth)
    )
      throw new Error('invalid status')
    ssoAccount.value = data as SSOAccount
  } catch {
    ssoError.value = true
  } finally {
    ssoLoading.value = false
  }
})

const emailRequestForm = useForm({
  currentPassword: '',
  email: props.pendingEmailChange?.newEmail || props.data.email || '',
})

const emailConfirmForm = useForm({
  code: '',
})

const passwordForm = useForm({
  currentPassword: '',
  password: '',
  passwordConfirmation: '',
})

const pendingEmailChange = computed(() => props.pendingEmailChange || null)
const now = ref(Date.now())
let expiryTimer: ReturnType<typeof setTimeout> | undefined
const emailCodeExpired = computed(() => {
  if (!pendingEmailChange.value) return false
  const expiresAt = Date.parse(pendingEmailChange.value.expiresAt)
  return !Number.isFinite(expiresAt) || expiresAt <= now.value
})

const scheduleExpiry = () => {
  clearTimeout(expiryTimer)
  now.value = Date.now()
  if (!pendingEmailChange.value) return
  const remaining = Date.parse(pendingEmailChange.value.expiresAt) - now.value
  if (remaining > 0) {
    expiryTimer = setTimeout(scheduleExpiry, Math.min(remaining, 2_147_483_647))
  }
}
onMounted(scheduleExpiry)
watch(
  () => [props.pendingEmailChange?.newEmail, props.pendingEmailChange?.expiresAt],
  () => {
    emailConfirmForm.reset('code')
    scheduleExpiry()
  },
)
watch(emailCodeExpired, (expired) => {
  if (expired) emailConfirmForm.reset('code')
})
onUnmounted(() => clearTimeout(expiryTimer))

const passwordConfirmationError = computed(() => {
  if (
    passwordForm.passwordConfirmation &&
    passwordForm.password !== passwordForm.passwordConfirmation
  ) {
    return t('Пароли не совпадают')
  }
  return null
})

const isPasswordFormValid = computed(() => {
  return (
    passwordForm.currentPassword.length > 0 &&
    passwordForm.password.length >= 8 &&
    passwordForm.password === passwordForm.passwordConfirmation &&
    !passwordConfirmationError.value
  )
})

const sendEmailCode = () => {
  if (!authmail.value || emailRequestForm.processing || emailConfirmForm.processing) return
  emailRequestForm.post('/auth/profile/settings/email/request', {
    preserveScroll: true,
    onFinish: () => {
      emailRequestForm.defaults('currentPassword', '')
      emailRequestForm.reset('currentPassword')
      emailConfirmForm.defaults('code', '')
      emailConfirmForm.reset('code')
    },
  })
}

const confirmEmail = () => {
  scheduleExpiry()
  if (
    !authmail.value ||
    !pendingEmailChange.value ||
    emailCodeExpired.value ||
    emailConfirmForm.processing ||
    emailRequestForm.processing
  )
    return
  emailConfirmForm.post('/auth/profile/settings/email/confirm', {
    preserveScroll: true,
    onFinish: () => {
      emailConfirmForm.defaults('code', '')
      emailConfirmForm.reset('code')
    },
  })
}

const changePassword = () => {
  if (passwordForm.processing || !isPasswordFormValid.value) return
  passwordForm.post('/auth/profile/settings/password', {
    preserveScroll: true,
    onFinish: () => {
      passwordForm.defaults({ currentPassword: '', password: '', passwordConfirmation: '' })
      passwordForm.reset('currentPassword', 'password', 'passwordConfirmation')
    },
  })
}

const formatDateTime = (value: string) => {
  if (!value) return '—'
  const date = new Date(value)
  return date.toLocaleString(locale.value === 'en' ? 'en-GB' : 'ru-RU', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
</script>
