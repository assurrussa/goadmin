<template>
  <section class="rounded-xl border border-border-secondary p-6 space-y-4">
    <h2>{{ t('mfa.localTitle', 'Двухфакторная защита локальной учётной записи') }}</h2>
    <p>
      {{
        t(
          'mfa.scope',
          'Эта настройка относится к входу в эту админку. Защита входа через SSO управляется отдельно у провайдера.',
        )
      }}
    </p>
    <p v-if="state === 'unavailable' || operationUnavailable">
      {{
        t(
          'mfa.unavailable',
          'Локальная настройка недоступна для этого способа входа или конфигурации сервиса.',
        )
      }}
    </p>
    <p v-else-if="state === 'unknown' || uncertain" role="alert">
      {{
        t(
          'mfa.unknown',
          'Состояние защиты неизвестно. Войдите заново и проверьте настройки перед повторной операцией.',
        )
      }}
    </p>
    <p v-else-if="!signedOut && !signinRequired">
      {{ enabled ? t('mfa.enabled', 'Защита включена') : t('mfa.disabled', 'Защита не включена') }}
    </p>
    <p v-if="message" role="status">{{ message?.() }}</p>
    <template v-if="manageable && !signedOut && !uncertain">
      <PasswordInput
        v-model="password"
        name="mfaPassword"
        :label="t('Текущий пароль')"
        autocomplete="current-password"
      />
      <FormInput
        v-if="enabled || enrollment"
        v-model="code"
        name="mfaCode"
        :label="t('Код приложения или резервный код')"
        autocomplete="one-time-code"
      />
      <template v-if="enrollment">
        <p>{{ t('Добавьте секрет в приложение-аутентификатор. Он показан только сейчас.') }}</p>
        <img
          v-if="qr"
          :src="qr"
          :alt="t('mfa.qr', 'QR-код для приложения-аутентификатора')"
          width="240"
          height="240"
        />
        <p class="break-all select-all">{{ enrollment.secret }}</p>
        <p>
          {{
            t(
              'mfa.confirmHint',
              'Введите код нового приложения. До подтверждения защита не меняется.',
            )
          }}
        </p>
        <div class="flex flex-wrap items-center gap-3" data-mfa-actions>
          <FormButton :disabled="busy" @click="run('confirm')">{{
            t('Подтвердить новый код')
          }}</FormButton>
          <FormButton variant="secondary" :disabled="busy" @click="run('cancel')">{{
            t('Отмена')
          }}</FormButton>
        </div>
      </template>
      <div v-else class="flex flex-wrap items-center gap-3" data-mfa-actions>
        <FormButton :disabled="busy" @click="run('begin')">{{
          enabled ? t('Заменить приложение') : t('Включить защиту')
        }}</FormButton>
        <FormButton v-if="enabled" :disabled="busy" @click="run('recovery')">{{
          t('Новые резервные коды')
        }}</FormButton>
        <FormButton v-if="enabled" :disabled="busy" @click="run('disable')">{{
          t('Отключить защиту')
        }}</FormButton>
      </div>
    </template>
    <template v-if="recoveryCodes.length">
      <p>
        {{
          t(
            'Сохраните резервные коды. Каждый действует один раз. После ухода со страницы они больше не показываются.',
          )
        }}
      </p>
      <ul>
        <li v-for="item in recoveryCodes" :key="item" class="select-all">{{ item }}</li>
      </ul>
    </template>
    <a v-if="signedOut || signinRequired" href="/auth/login">{{ t('Войти заново') }}</a>
  </section>
</template>
<script setup lang="ts">
import { bindEphemeralAuthPage } from '@/lib/ephemeral-auth'
import axios from 'axios'
import QRCode from 'qrcode'
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n, registerTranslations } from '@/i18n'
import PasswordInput from '@/components/form/PasswordInput.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
registerTranslations({
  ru: {},
  en: {
    'mfa.errorUnavailable':
      'Local MFA management is unavailable. This does not mean protection is disabled.',
    'mfa.errorSignin': 'Your session has ended or changed. Sign in again before another operation.',
    'mfa.errorRate': 'Too many verification attempts. Wait before trying again.',
    'mfa.errorProof':
      'Check your current password and a fresh authenticator or unused recovery code.',
    'mfa.errorRejected':
      'Verification was rejected. Check fresh proof; if setup or session expired, sign in again.',
    'mfa.errorUnknown':
      'The service could not confirm the outcome. Sign in again and check protection before repeating.',

    'mfa.localTitle': 'Local account two-factor protection',
    'mfa.scope':
      'This setting protects local admin sign-in. SSO protection is managed separately by your provider.',
    'mfa.unavailable':
      'Local setup is unavailable for this sign-in method or service configuration.',
    'mfa.unknown':
      'Protection status is unknown. Sign in again and check settings before another operation.',
    'mfa.enabled': 'Protection enabled',
    'mfa.disabled': 'Protection disabled',
    'mfa.qr': 'Authenticator setup QR code',
    'mfa.confirmHint': 'Enter a code from the new app. Protection changes only after confirmation.',
    'Текущий пароль': 'Current password',
    'Код приложения или резервный код': 'Authenticator or recovery code',
    'Добавьте секрет в приложение-аутентификатор. Он показан только сейчас.':
      'Add the secret to your authenticator. It is shown only now.',
    'Подтвердить новый код': 'Confirm new code',
    Отмена: 'Cancel',
    'Заменить приложение': 'Replace authenticator',
    'Включить защиту': 'Enable protection',
    'Новые резервные коды': 'New recovery codes',
    'Отключить защиту': 'Disable protection',
    'Войти заново': 'Sign in again',
    'Настройки сохранены. Все сеансы завершены.': 'Settings saved. All sessions have ended.',
    'Настройка отменена.': 'Setup cancelled.',
    'Введите код нового приложения для подтверждения.': 'Enter a code from the new app to confirm.',
    'Не удалось выполнить операцию. Проверьте пароль и код. После потери ответа войдите заново и проверьте состояние защиты.':
      'Operation failed. Check the password and code. If the response was lost, sign in again and check protection status.',
    'Сохраните резервные коды. Каждый действует один раз. После ухода со страницы они больше не показываются.':
      'Save the recovery codes. Each works once. They will not be shown again after leaving this page.',
  },
})
const props = defineProps<{ state: 'unavailable' | 'unknown' | 'enabled' | 'disabled' }>()
const { t } = useI18n()
const enabled = computed(() => props.state === 'enabled')
const operationUnavailable = ref(false)
const signinRequired = ref(false)
const manageable = computed(
  () =>
    !operationUnavailable.value &&
    !signinRequired.value &&
    (props.state === 'enabled' || props.state === 'disabled'),
)
const uncertain = ref(false)
const qr = ref('')
const password = ref('')
const code = ref('')
const busy = ref(false)
const message = ref<(() => string) | null>(null)
const signedOut = ref(false)
const enrollment = ref<{ id: string; secret: string; url: string } | null>(null)
const recoveryCodes = ref<string[]>([])
function clearSecrets() {
  password.value = ''
  code.value = ''
  qr.value = ''
  enrollment.value = null
  recoveryCodes.value = []
}
const lifecycle = bindEphemeralAuthPage(clearSecrets)
onBeforeUnmount(lifecycle.dispose)
async function run(action: string) {
  if (!lifecycle.isActive() || busy.value || uncertain.value || !manageable.value) return
  busy.value = true
  message.value = null
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
          typeof data.enrollment.secret !== 'string' ||
          typeof data.enrollment.url !== 'string' ||
          !data.enrollment.url.startsWith('otpauth://totp/'))) ||
      ((action === 'confirm' || action === 'recovery') &&
        (!Array.isArray(data.recoveryCodes) ||
          !data.recoveryCodes.length ||
          !data.recoveryCodes.every((item: unknown) => typeof item === 'string'))) ||
      ((action === 'confirm' || action === 'recovery' || action === 'disable') && !data.signedOut)
    )
      throw new Error('Invalid MFA response')
    const nextQR = data.enrollment
      ? await QRCode.toDataURL(data.enrollment.url, { width: 240, margin: 4 })
      : ''
    if (!lifecycle.isActive()) return
    qr.value = nextQR
    enrollment.value = data.enrollment || null
    recoveryCodes.value = data.recoveryCodes || []
    signedOut.value = Boolean(data.signedOut)
    message.value = () =>
      signedOut.value
        ? t('Настройки сохранены. Все сеансы завершены.')
        : action === 'cancel'
          ? t('Настройка отменена.')
          : t('Введите код нового приложения для подтверждения.')
  } catch (error) {
    if (lifecycle.isActive()) {
      const response = axios.isAxiosError(error) ? error.response : undefined
      const status = response?.status
      const reason = typeof response?.data?.code === 'string' ? response.data.code : ''
      if (reason === 'mfa_unavailable') {
        operationUnavailable.value = true
        clearSecrets()
        message.value = () =>
          t(
            'mfa.errorUnavailable',
            'Локальная настройка MFA недоступна. Это не означает, что защита выключена.',
          )
      } else if (status === 401 || status === 403 || reason === 'signin_required') {
        signinRequired.value = true
        clearSecrets()
        message.value = () =>
          t(
            'mfa.errorSignin',
            'Сеанс завершён или изменился. Войдите заново перед следующей операцией.',
          )
      } else if (status === 429 && reason === 'rate_limited') {
        message.value = () =>
          t('mfa.errorRate', 'Слишком много попыток проверки. Подождите перед новой попыткой.')
      } else if (status === 400 && reason === 'proof_invalid') {
        message.value = () =>
          t(
            'mfa.errorProof',
            'Проверьте текущий пароль и свежий код приложения или неиспользованный резервный код.',
          )
      } else if (status === 400 && reason === 'verification_rejected') {
        message.value = () =>
          t(
            'mfa.errorRejected',
            'Проверка отклонена. Проверьте свежие пароль и код; если настройка или сеанс истекли, войдите заново.',
          )
      } else {
        uncertain.value = true
        clearSecrets()
        message.value = () =>
          t(
            'mfa.errorUnknown',
            'Сервис не подтвердил результат. Войдите заново и проверьте защиту перед повторной операцией.',
          )
      }
    }
  } finally {
    password.value = ''
    code.value = ''
    busy.value = false
  }
}
</script>
