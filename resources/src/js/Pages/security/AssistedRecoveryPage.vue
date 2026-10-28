<script setup lang="ts">
import { adminFetch, AdminFetchNotSentError } from '@/http'
import { ref, onBeforeUnmount } from 'vue'
import { router } from '@inertiajs/vue3'
import { useI18n, registerTranslations } from '@/i18n'
import { bindEphemeralAuthPage } from '@/lib/ephemeral-auth'
import AppHead from '@/components/layout/AppHead.vue'
registerTranslations({
  ru: {},
  en: {
    'Восстановление доступа': 'Account recovery',
    'Найти по текущему email': 'Find by current email',
    Поиск: 'Search',
    'Выберите пользователя': 'Select a user',
    'Новый email': 'New email',
    Причина: 'Reason',
    'Ссылка на независимую проверку (без секретов)':
      'Independent verification reference (no secrets)',
    'Личность проверена независимо': 'Identity verified independently',
    'Ваш текущий пароль': 'Your current password',
    'Ваш код приложения или резервный код': 'Your authenticator or recovery code',
    'Отправить приглашение': 'Send invitation',
    'Проверьте личность по доверенным данным или известному каналу. Имени и нового email недостаточно.':
      'Verify identity using trusted records or a known channel. A name and new email are insufficient.',
    'Приглашение поставлено в очередь. Данные изменятся только после завершения восстановления.':
      'Invitation queued. Credentials change only after recovery is completed.',
    'Результат неизвестен. Не отправляйте новое приглашение до проверки владельцем сервиса.':
      'Outcome unknown. Do not send another invitation until the service owner reconciles this request.',
    'Приглашение не отправлено: сеанс не готов. Введите пароль и MFA заново и повторите попытку.':
      'Invitation not sent: the session is not ready. Re-enter your password and MFA and try again.',
    'Запрос отклонён. Проверьте разрешение, независимую проверку личности и свежие пароль и MFA.':
      'Request rejected. Check permission, independent identity verification, and fresh password and MFA.',
    'Учётная запись с таким email не найдена.': 'No account found with this email.',
    'Введите полный текущий email. Поиск по логину не поддерживается.':
      'Enter the full current email. Username search is not supported.',
    'Здесь можно начать восстановление другого пользователя. Это не список поданных заявок.':
      'Start recovery for another user here. This is not a list of submitted requests.',
    'Введите существующий email учётной записи.': 'Enter the account’s existing email address.',
  },
})
const props = defineProps<{
  title: string
  nativeCSRF: string
  search: string
  searchInvalid?: boolean
  targets: { subject: string; name: string; email: string }[]
}>()
const { t } = useI18n()
const query = ref(props.search)
const subject = ref('')
const password = ref('')
const code = ref('')
const locked = ref(false)
const message = ref<(() => string) | null>(null)
const lifecycle = bindEphemeralAuthPage(() => {
  password.value = ''
  code.value = ''
})
onBeforeUnmount(lifecycle.dispose)
function searchTargets() {
  router.get('/account-recovery', { search: query.value }, { replace: true })
}
async function submit(event: Event) {
  if (locked.value || !lifecycle.isActive()) return
  locked.value = true
  const form = event.target as HTMLFormElement
  try {
    const response = await adminFetch('/account-recovery', {
      method: 'POST',
      credentials: 'same-origin',
      mode: 'same-origin',
      redirect: 'error',
      cache: 'no-store',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'X-CSRF-Token': props.nativeCSRF,
      },
      body: new URLSearchParams(new FormData(form) as unknown as Record<string, string>),
    })
    if (!lifecycle.isActive()) return
    message.value = () =>
      response.status >= 500
        ? t(
            'Результат неизвестен. Не отправляйте новое приглашение до проверки владельцем сервиса.',
          )
        : response.ok
          ? t(
              'Приглашение поставлено в очередь. Данные изменятся только после завершения восстановления.',
            )
          : t(
              'Запрос отклонён. Проверьте разрешение, независимую проверку личности и свежие пароль и MFA.',
            )
  } catch (error) {
    if (lifecycle.isActive() && error instanceof AdminFetchNotSentError) {
      locked.value = false
      message.value = () =>
        t(
          'Приглашение не отправлено: сеанс не готов. Введите пароль и MFA заново и повторите попытку.',
        )
    } else if (lifecycle.isActive())
      message.value = () =>
        t('Результат неизвестен. Не отправляйте новое приглашение до проверки владельцем сервиса.')
  } finally {
    password.value = ''
    code.value = ''
  }
}
</script>
<template>
  <AppHead :title="t('Восстановление доступа')" />
  <section class="max-w-3xl space-y-6 rounded-xl border border-border-secondary bg-surface p-6">
    <h1 class="text-2xl font-semibold">{{ t('Восстановление доступа') }}</h1>
    <p>
      {{
        t(
          'Проверьте личность по доверенным данным или известному каналу. Имени и нового email недостаточно.',
        )
      }}
    </p>
    <p>{{ t('Введите существующий email учётной записи.') }}</p>
    <form class="flex gap-3" @submit.prevent="searchTargets">
      <label class="flex-1"
        >{{ t('Найти по текущему email')
        }}<input
          v-model="query"
          type="email"
          required
          placeholder="name@example.com"
          maxlength="256"
          class="block w-full rounded border p-2"
      /></label>
      <button type="submit" :disabled="locked" class="underline">{{ t('Поиск') }}</button>
    </form>
    <p>
      {{
        t('Здесь можно начать восстановление другого пользователя. Это не список поданных заявок.')
      }}
    </p>
    <p v-if="props.searchInvalid">
      {{ t('Введите полный текущий email. Поиск по логину не поддерживается.') }}
    </p>
    <p v-else-if="props.search && !targets.length">
      {{ t('Учётная запись с таким email не найдена.') }}
    </p>
    <form v-if="targets.length > 0" class="space-y-4" @submit.prevent="submit">
      <fieldset :disabled="locked" class="space-y-4">
        <label class="block"
          >{{ t('Выберите пользователя')
          }}<select
            v-model="subject"
            name="subject"
            required
            class="block w-full rounded border p-2"
          >
            <option value="" disabled>{{ t('Выберите пользователя') }}</option>
            <option v-for="target in targets" :key="target.subject" :value="target.subject">
              {{ target.name }} — {{ target.email }}
            </option>
          </select></label
        >
        <label class="block"
          >{{ t('Новый email')
          }}<input
            name="email"
            type="email"
            required
            autocomplete="off"
            class="block w-full rounded border p-2"
        /></label>
        <label class="block"
          >{{ t('Причина')
          }}<input name="reason" required maxlength="512" class="block w-full rounded border p-2"
        /></label>
        <label class="block"
          >{{ t('Ссылка на независимую проверку (без секретов)')
          }}<input name="evidence" required maxlength="256" class="block w-full rounded border p-2"
        /></label>
        <label class="block"
          ><input type="checkbox" name="identity_verified" value="yes" required />
          {{ t('Личность проверена независимо') }}</label
        >
        <label class="block"
          >{{ t('Ваш текущий пароль')
          }}<input
            v-model="password"
            name="password"
            type="password"
            required
            autocomplete="current-password"
            class="block w-full rounded border p-2"
        /></label>
        <label class="block"
          >{{ t('Ваш код приложения или резервный код')
          }}<input
            v-model="code"
            name="code"
            maxlength="128"
            autocomplete="off"
            class="block w-full rounded border p-2"
        /></label>
        <button
          type="submit"
          :disabled="!subject"
          class="rounded bg-primary px-4 py-2 text-primary-foreground"
        >
          {{ t('Отправить приглашение') }}
        </button>
      </fieldset>
      <p role="status">{{ message?.() }}</p>
    </form>
  </section>
</template>
