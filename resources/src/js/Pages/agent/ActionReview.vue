<script setup lang="ts">
import { ref } from 'vue'
import AppHead from '@/components/layout/AppHead.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { adminFetch } from '@/http'
import { t } from '@/i18n'

const props = defineProps<{
  title: string
  description: string
  confirmLabel: string
  path: string
  requestID: string
  status: string
  expiresAt: string
  recordID: string
  revision: string
  fields: { label: string; value: string }[] | null
  result: Record<string, string> | null
  receiptLink?: { href: string; label: string } | null
}>()
const busy = ref(false)
const status = ref(props.status)
const result = ref(props.result)
const receiptLink = ref(props.receiptLink)
const notice = ref('')
const labels: Record<string, string> = {
  succeeded: 'Action completed.',
  declined: 'Proposal declined.',
  expired: 'Proposal expired.',
  conflict: 'The record changed. Create a new proposal to review the current version.',
}
async function decide(decision: 'approve' | 'decline') {
  if (busy.value || status.value !== 'pending') return
  busy.value = true
  notice.value = ''
  try {
    const response = await adminFetch(props.path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ decision }),
      redirect: 'error',
    })
    if (!response.ok) throw new Error()
    const data = await response.json()
    if (data.request_id !== props.requestID || !Object.hasOwn(labels, data.status))
      throw new Error()
    status.value = data.status
    receiptLink.value = data.receiptLink
    result.value = data.result
  } catch {
    status.value = 'unknown'
    notice.value = 'Reload this page to check the original request.'
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <AppHead :title="t(title)" />
  <section class="max-w-3xl space-y-5 rounded-xl border border-border-primary bg-card p-6">
    <h1 class="text-xl font-semibold">{{ t(title) }}</h1>
    <template v-if="status === 'pending'">
      <p>{{ t(description) }}</p>
      <dl class="space-y-3">
        <div v-for="field in fields" :key="field.label">
          <dt class="text-sm text-text-secondary">{{ t(field.label) }}</dt>
          <dd class="whitespace-pre-wrap break-words">{{ field.value }}</dd>
        </div>
      </dl>
      <p class="text-sm text-text-secondary">
        {{ t('Record') }} {{ recordID }} · {{ t('Revision') }} {{ revision }}
      </p>
      <p>
        {{ t('Confirm before') }}
        <time :datetime="expiresAt">{{ new Date(expiresAt).toLocaleString() }}</time>
      </p>
      <div class="flex flex-wrap gap-3">
        <AppButton :disabled="busy" @click="decide('approve')">{{ t(confirmLabel) }}</AppButton>
        <AppButton variant="secondary" :disabled="busy" @click="decide('decline')">{{
          t('Decline proposal')
        }}</AppButton>
      </div>
    </template>
    <p v-else-if="labels[status]" role="status">{{ t(labels[status]) }}</p>
    <dl v-if="status === 'succeeded'">
      <div v-for="(value, key) in result" :key="key">
        <dt>{{ t(String(key)) }}</dt>
        <dd>{{ value }}</dd>
      </div>
    </dl>
    <a v-if="status === 'succeeded' && receiptLink" :href="receiptLink.href" class="underline">
      {{ t(receiptLink.label) }}
    </a>
    <p v-if="notice" role="alert">{{ t(notice) }}</p>
    <p class="break-all text-sm text-text-secondary">UUID {{ requestID }}</p>
  </section>
</template>
