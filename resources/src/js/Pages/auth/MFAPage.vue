<template>
  <AuthLayout>
    <template #title>{{ t('Подтверждение входа') }}</template>
    <template #subtitle>{{ t('Введите код приложения или резервный код') }}</template>
    <AdminForm :form="form" :fields="['code']" @submit="submit">
      <FormInput
        v-model="form.code"
        name="code"
        :label="t('Код подтверждения')"
        autocomplete="one-time-code"
        required
        :error="form.errors.code"
      />
      <FormButton
        type="submit"
        :loading="form.processing"
        :disabled="form.processing || !form.code"
        full-width
        >{{ t('Подтвердить') }}</FormButton
      >
    </AdminForm>
    <template #footer
      ><Link href="/auth/login">{{ t('Вернуться ко входу') }}</Link></template
    >
  </AuthLayout>
</template>
<script setup lang="ts">
import { t } from '@/i18n'

import { bindEphemeralAuthPage } from '@/lib/ephemeral-auth'
import { onBeforeUnmount } from 'vue'
import { Link, useForm } from '@inertiajs/vue3'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import AdminForm from '@/components/form/AdminForm.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
const props = defineProps<{ challengeID: string }>()
const form = useForm({ code: '', challengeID: props.challengeID })
const lifecycle = bindEphemeralAuthPage(() => form.reset('code'))
onBeforeUnmount(lifecycle.dispose)
function submit() {
  if (!lifecycle.isActive() || form.processing) return
  form.post('/auth/mfa', { onFinish: () => form.reset('code') })
}
</script>
