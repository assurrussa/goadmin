<script setup lang="ts">
import { ref } from 'vue'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { adminFetch } from '@/http'
import { t } from '@/i18n'

const props = defineProps<{ grants: string[] }>()
const connections = ref(props.grants)
const code = ref('')
const busy = ref(false)
const notice = ref('')
const message = ref('')
type Decision = 'approve' | 'deny' | 'revoke'

async function decide(decision: Decision, value: string) {
  if (busy.value) return
  busy.value = true
  notice.value = message.value = ''
  try {
    const response = await adminFetch('/agent/product-events/connect', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
      body: JSON.stringify({ code: value.trim().toUpperCase(), decision }),
      redirect: 'error',
    })
    if (response.status === 404 || response.status === 422) {
      message.value = 'The code is invalid, expired or already used.'
      return
    }
    if (!response.ok) throw new Error()
    connections.value = (await response.json()).grants
    notice.value = decision === 'approve' ? 'Reading allowed.' : 'Access closed.'
    code.value = ''
  } catch {
    message.value = 'Reload this page to check connection status.'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <AppHead :title="t('Product journal connections')" />
  <PageActionBar
    back-href="/product-events"
    :back-text="t('Журнал действий с товарами')"
    :label="t('Product journal connections')"
    :use-history="false"
  />
  <section class="max-w-xl space-y-5 rounded-xl border border-border-primary bg-card p-6">
    <p>
      {{
        t(
          'Enter the code from your local MCP client to allow it to read your product events and their unpublish reasons.',
        )
      }}
    </p>
    <p class="text-sm text-text-secondary">
      {{ t('Access lasts up to 15 minutes and ends when you sign out.') }}
    </p>
    <form class="space-y-4" @submit.prevent="decide('approve', code)">
      <FormInput
        v-model="code"
        name="code"
        label="Connection code"
        autocomplete="off"
        placeholder="ABCDE-FGHIJ"
        :required="true"
      />
      <div class="flex flex-wrap gap-3">
        <AppButton type="submit" :disabled="busy || !code.trim()">{{
          t('Allow reading')
        }}</AppButton>
        <AppButton
          type="button"
          variant="secondary"
          :disabled="busy || !code.trim()"
          @click="decide('deny', code)"
          >{{ t('Deny') }}</AppButton
        >
      </div>
    </form>
    <p v-if="notice" role="status">{{ t(notice) }}</p>
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
