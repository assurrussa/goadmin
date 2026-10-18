<script setup lang="ts">
import { computed, ref } from 'vue'
import { router } from '@inertiajs/vue3'
import { t, registerTranslations, formatDateTime } from '@/i18n'
import AppHead from '@/components/layout/AppHead.vue'
import AppButton from '@/components/ui/AppButton.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'

registerTranslations({
  ru: {
    'SSO access': 'Доступ через SSO', 'Admission subject': 'Subject ID пользователя в SSO',
    'Admission roles': 'Разрешённые к выдаче роли', 'Admission expires': 'Действует до',
    'Admission invite': 'Создать приглашение', 'Admission approve': 'Одобрить доступ',
    'Admission reject': 'Отклонить', 'Admission revoke': 'Отозвать приглашение',
    'Admission pending': 'Ожидает одобрения', 'Admission approved': 'Одобрено — нужен новый SSO-вход',
    'Admission active': 'Доступ выдан', 'Admission rejected': 'Отклонено',
    'Admission invitations': 'Приглашения', 'Admission requests': 'Заявки на доступ',
    'Admission empty': 'Записей пока нет', 'Admission revoked': 'Отозвано',
    'Admission accepted': 'Принято', 'Admission open': 'Ожидает принятия',
    'Admission error': 'Не удалось выполнить действие. Проверьте права и актуальность данных.',
    'Admission explanation': 'Приглашение не заменяет доступ к проекту в SSO. После одобрения нужен новый вход.',
    'Admission limit': 'Показаны последние 100 записей.',
    'Admission role label': 'Роль',
    'Admission confirm reject': 'Отклонить эту заявку?',
    'Admission confirm revoke': 'Отозвать это приглашение?',
    'Admission expired': 'Срок истёк',
  },
  en: {
    'SSO access': 'SSO access', 'Admission subject': 'User subject ID in SSO',
    'Admission roles': 'Roles you may grant', 'Admission expires': 'Expires at',
    'Admission invite': 'Create invitation', 'Admission approve': 'Approve access',
    'Admission reject': 'Reject', 'Admission revoke': 'Revoke invitation',
    'Admission pending': 'Awaiting approval', 'Admission approved': 'Approved — sign in through SSO again',
    'Admission active': 'Access granted', 'Admission rejected': 'Rejected',
    'Admission invitations': 'Invitations', 'Admission requests': 'Access requests',
    'Admission empty': 'No records yet', 'Admission revoked': 'Revoked',
    'Admission accepted': 'Accepted', 'Admission open': 'Awaiting acceptance',
    'Admission error': 'Action failed. Check permissions and refresh the current state.',
    'Admission explanation': 'An invitation does not replace SSO project access. A new sign-in is required after approval.',
    'Admission limit': 'Showing the latest 100 records.',
    'Admission role label': 'Role',
    'Admission confirm reject': 'Reject this request?',
    'Admission confirm revoke': 'Revoke this invitation?',
    'Admission expired': 'Expired',
  },
})
type Role = { ID: number; PublicID: string; Name?: string }
type RequestRow = { ID: string; ExternalSubject: string; Status: string; Version: number }
type Invitation = { ID: string; ExternalSubject: string; ExpiresAt: string; Revoked: boolean; AcceptedSubject: string }
const props = defineProps<{
  records: RequestRow[] | null; invitations: Invitation[] | null;
  inviteRoles: Role[] | null; approveRoles: Role[] | null;
  canReject: boolean; canInvite: boolean;
}>()
const subject = ref('')
const selectedInvite = ref<number[]>([])
const selectedApproval = ref<Record<string, number[]>>({})
const expires = ref('')
const busy = ref(false)
const error = ref('')
const requests = computed(() => props.records ?? [])
const invitations = computed(() => props.invitations ?? [])
const inputClass = 'rounded-lg border border-border-primary bg-surface p-2 text-text-primary'
function until() {
  const parsed = new Date(expires.value)
  return Number.isNaN(parsed.getTime()) ? '' : parsed.toISOString()
}
function send(path: string, body: Record<string, string | number | number[]>) {
  if (busy.value) return
  error.value = ''
  busy.value = true
  router.post(path, body, {
    preserveScroll: true,
    onError: () => { error.value = t('Admission error') },
    onFinish: () => { busy.value = false },
  })
}
function invite() {
  if (!until()) { error.value = t('Admission error'); return }
  send('/admissions/invitations', { subject: subject.value, roleIds: selectedInvite.value, expiresAt: until() })
}
function approve(row: RequestRow) {
  if (!until()) { error.value = t('Admission error'); return }
  send('/admissions/approve', { subject: row.ExternalSubject, version: row.Version,
    roleIds: selectedApproval.value[row.ID] ?? [], expiresAt: until() })
}
function reject(row: RequestRow) {
  if (window.confirm(t('Admission confirm reject')))
    send('/admissions/reject', { subject: row.ExternalSubject, version: row.Version })
}
function revoke(row: Invitation) {
  if (window.confirm(t('Admission confirm revoke'))) send('/admissions/revoke', { id: row.ID })
}
function invitationState(row: Invitation) {
  if (row.Revoked) return 'Admission revoked'
  if (row.AcceptedSubject) return 'Admission accepted'
  if (new Date(row.ExpiresAt).getTime() <= Date.now()) return 'Admission expired'
  return 'Admission open'
}
</script>
<template>
  <AppHead :title="t('SSO access')" />
  <div class="space-y-6">
    <PageActionBar back-href="/" :label="t('SSO access')" />
    <p class="text-sm text-text-secondary">{{ t('Admission explanation') }}</p>
    <p v-if="error" role="alert" class="text-danger">{{ error }}</p>
    <section class="rounded-xl border border-border-primary bg-surface p-4 space-y-4">
      <h2 class="font-semibold">{{ t('Admission invite') }}</h2>
      <FormInput v-model="subject" name="subject" :label="t('Admission subject')" :maxlength="512" />
      <label class="block space-y-2">
        <span>{{ t('Admission roles') }}</span>
        <select v-model="selectedInvite" multiple :class="inputClass" class="w-full" :disabled="busy || !canInvite">
          <option v-for="role in inviteRoles ?? []" :key="role.ID" :value="role.ID">
            {{ role.Name ?? t('Admission role label') + ' ' + role.ID }}
          </option>
        </select>
      </label>
      <label class="block space-y-2">
        <span>{{ t('Admission expires') }}</span>
        <input v-model="expires" type="datetime-local" :class="inputClass" required />
      </label>
      <AppButton :disabled="busy || !canInvite || !subject || !selectedInvite.length || !expires" @click="invite">
        {{ t('Admission invite') }}
      </AppButton>
    </section>
    <section class="space-y-3">
      <h2 class="font-semibold">{{ t('Admission requests') }}</h2>
      <p class="text-xs text-text-secondary">{{ t('Admission limit') }}</p>
      <p v-if="!requests.length">{{ t('Admission empty') }}</p>
      <article v-for="row in requests" :key="row.ID" class="rounded-xl border border-border-primary bg-surface p-4 space-y-3">
        <p class="break-all">{{ row.ExternalSubject }}</p>
        <p>{{ t('Admission ' + row.Status) }}</p>
        <div v-if="row.Status === 'pending'" class="flex flex-wrap items-center gap-3">
          <label class="space-y-2">
            <span class="block">{{ t('Admission roles') }}</span>
            <select v-model="selectedApproval[row.ID]" multiple :class="inputClass" :disabled="busy">
              <option v-for="role in approveRoles ?? []" :key="role.ID" :value="role.ID">
                {{ role.Name ?? t('Admission role label') + ' ' + role.ID }}
              </option>
            </select>
          </label>
          <AppButton :disabled="busy || !(selectedApproval[row.ID]?.length) || !expires" @click="approve(row)">
            {{ t('Admission approve') }}
          </AppButton>
        </div>
        <AppButton v-if="canReject && ['pending', 'approved'].includes(row.Status)" variant="outline"
          :disabled="busy" @click="reject(row)">{{ t('Admission reject') }}</AppButton>
      </article>
    </section>
    <section class="space-y-3">
      <h2 class="font-semibold">{{ t('Admission invitations') }}</h2>
      <p v-if="!invitations.length">{{ t('Admission empty') }}</p>
      <article v-for="row in invitations" :key="row.ID" class="rounded-xl border border-border-primary bg-surface p-4 space-y-2">
        <p class="break-all">{{ row.ExternalSubject }}</p>
        <p>{{ t(invitationState(row)) }} · {{ formatDateTime(row.ExpiresAt) }}</p>
        <p class="text-xs break-all">{{ row.ID }}</p>
        <AppButton v-if="canInvite && !row.Revoked && !row.AcceptedSubject" variant="outline"
          :disabled="busy" @click="revoke(row)">{{ t('Admission revoke') }}</AppButton>
      </article>
    </section>
  </div>
</template>
