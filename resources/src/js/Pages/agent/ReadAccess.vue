<script setup lang="ts">
import { ref } from 'vue'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { adminFetch } from '@/http'
import { t } from '@/i18n'

const props = defineProps<{
  grants: string[]
  connectPath: string
  title: string
  description: string
  backHref?: string
  backText?: string
  confirmedAction?: boolean
}>()
const connections = ref(props.grants ?? [])
const code = ref('')
const busy = ref(false)
type Decision = 'approve' | 'deny' | 'revoke'
const lastDecision = ref<Decision>()
const message = ref('')

async function decide(decision: Decision, value: string) {
  if (busy.value) return
  busy.value = true
  lastDecision.value = undefined
  message.value = ''
  try {
    const response = await adminFetch(props.connectPath, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify(
        props.confirmedAction && decision === 'revoke'
          ? { connection_id: value, decision }
          : { code: value.trim().toUpperCase(), decision },
      ),
      redirect: 'error',
    })
    if (response.status === 404 || response.status === 422) {
      message.value = 'The code is invalid, expired or already used.'
      return
    }
    if (!response.ok) throw new Error()
    connections.value = (await response.json()).grants
    lastDecision.value = decision
    code.value = ''
  } catch {
    message.value = 'Reload this page to check connection status.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AppHead :title="t(title)" />
  <PageActionBar
    v-if="backHref"
    :back-href="backHref"
    :back-text="backText && t(backText)"
    :label="t(title)"
    :use-history="false"
  />
  <section class="max-w-xl space-y-5 rounded-xl border border-border-primary bg-card p-6">
    <h1 v-if="!backHref" class="text-lg font-semibold">{{ t(title) }}</h1>
    <p>{{ t(description) }}</p>
    <p v-if="confirmedAction">
      {{ t('Each proposal requires your confirmation in this browser.') }}
    </p>
    <p class="text-sm text-text-secondary">
      {{ t('Access lasts up to 15 minutes and ends when you sign out.') }}
    </p>
    <form class="space-y-4" @submit.prevent="decide('approve', code)">
      <FormInput
        v-model="code"
        name="code"
        :label="confirmedAction ? 'Code' : 'Connection code'"
        autocomplete="off"
        placeholder="ABCDE-FGHIJ"
        :required="true"
      />
      <div class="flex flex-wrap gap-3">
        <AppButton type="submit" :disabled="busy || !code.trim()">{{
          t(confirmedAction ? 'Connect' : 'Allow reading')
        }}</AppButton>
        <AppButton
          v-if="!confirmedAction"
          type="button"
          variant="secondary"
          :disabled="busy || !code.trim()"
          @click="decide('deny', code)"
          >{{ t('Deny') }}</AppButton
        >
      </div>
    </form>
    <p v-if="lastDecision" role="status">
      <template v-if="lastDecision === 'approve'">
        {{ t(confirmedAction ? 'Connection approved.' : 'Reading allowed.') }}
        {{ t('Return to your MCP client and repeat the request.') }}
      </template>
      <template v-else>{{ t('Access closed.') }}</template>
    </p>
    <p v-if="message" role="alert" class="text-error">{{ t(message) }}</p>
    <section v-if="connections.length" class="space-y-3">
      <h2 class="font-semibold">{{ t('Active connections') }}</h2>
      <ul class="space-y-2">
        <li
          v-for="connection in connections"
          :key="connection"
          class="flex items-center justify-between gap-4"
        >
          <code>{{ connection }}</code>
          <AppButton
            type="button"
            variant="secondary"
            :disabled="busy"
            @click="decide('revoke', connection)"
            >{{ t('Revoke') }}</AppButton
          >
        </li>
      </ul>
    </section>
  </section>
</template>
