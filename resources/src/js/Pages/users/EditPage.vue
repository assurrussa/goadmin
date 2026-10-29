<template>
  <AppHead :title="props.title" />
  <RecordEditForm
    :form="form"
    :fields="['username', 'name', 'lastName', 'email']"
    :return-href="listHref"
    :label="t('Редактирование')"
    @submit="handleForm"
  >
    <FormInput
      v-model="form.name"
      name="name"
      type="text"
      :label="t('Имя')"
      required
      :error="form.errors.name"
    />
    <FormInput
      v-model="form.lastName"
      name="lastName"
      type="text"
      :label="t('Фамилия')"
      :error="form.errors.lastName"
    />
    <FormInput
      v-model="form.email"
      name="email"
      type="email"
      :label="t('core.ui.E-mail', 'E-mail')"
      required
      :error="form.errors.email"
      :readonly="!authmail"
    />
    <FormInput
      v-model="form.username"
      name="username"
      type="text"
      :label="t('core.ui.Nickname', 'Nickname')"
      :error="form.errors.username"
    />
  </RecordEditForm>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { useForm, usePage } from '@inertiajs/vue3'
import { watch } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import AppHead from '@/components/layout/AppHead.vue'
import FormInput from '@/components/form/FormInput.vue'
import RecordEditForm from '@/components/form/RecordEditForm.vue'
import { useRecordNavigation } from '@/composables/useRecordNavigation'

const { authmail } = useAdminCapabilities()
const basePath = '/users'
const { listHref } = useRecordNavigation(basePath)
const props = defineProps({
  title: { type: [String], default: undefined },
  old: { type: Object, default: () => ({}) },
  data: { type: Object, default: () => ({}) },
})

const page = usePage()
const oldData = (page.props.old as Record<string, string | null>) || {}

const form = useForm({
  name: oldData.name || props.data.name,
  email: (authmail.value && oldData.email) || props.data.email,
  lastName: oldData.lastName || props.data.lastName || null,
  username: oldData.username || props.data.username || null,
})

// Inertia can preserve this component while capabilities or canonical data change.
watch([authmail, () => props.data.email], ([enabled, email]) => {
  if (!enabled) form.email = email
})

function handleForm() {
  form.put(`${basePath}/${props.data.id}`, {
    // Email changes stay pending until confirmation; a later save must not resend them.
    onSuccess: () => {
      form.email = props.data.email
    },
  })
}
</script>
