<template>
  <AppHead :title="props.title" />
  <RecordEditForm
    :form="form"
    :fields="['username', 'name', 'lastName', 'email']"
    :return-href="listHref"
    :label="t('Редактирование администратора')"
    @submit="handleForm"
  >
    <!-- Name -->
    <FormInput
      v-model="form.name"
      name="name"
      type="text"
      :label="t('Имя')"
      autocomplete="given-name"
      required
      :error="form.errors.name"
    />

    <!-- Last Name -->
    <FormInput
      v-model="form.lastName"
      name="lastName"
      type="text"
      :label="t('Фамилия (необязательно)')"
      autocomplete="family-name"
      :error="form.errors.lastName"
    />

    <!-- Email -->
    <FormInput
      v-model="form.email"
      name="email"
      type="email"
      :label="t('Электронная почта')"
      required
      :error="form.errors.email"
      readonly
      help-text="Адрес связан с учётной записью. Администратор меняет свой email в настройках профиля с подтверждением пароля."
    />

    <!-- Username -->
    <FormInput
      v-model="form.username"
      name="username"
      type="text"
      :label="t('Имя пользователя (необязательно)')"
      autocomplete="username"
      help-text="Можно оставить пустым."
      :error="form.errors.username"
    />
  </RecordEditForm>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import FormInput from '@/components/form/FormInput.vue'
import RecordEditForm from '@/components/form/RecordEditForm.vue'
import { useRecordNavigation } from '@/composables/useRecordNavigation'

const basePath = '/admins'
const { listHref } = useRecordNavigation(basePath)
const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  old: {
    type: Object,
    default: () => ({}),
  },
  data: {
    type: Object,
    default: () => ({}),
  },
})

// Получаем данные из page props
const page = usePage()
const oldData = (page.props.old as Record<string, string | null> | undefined) ?? props.old
const oldValue = (field: string) =>
  Object.prototype.hasOwnProperty.call(oldData, field) ? oldData[field] : (props.data[field] ?? '')

const form = useForm({
  name: oldValue('name'),
  email: props.data.email ?? '',
  lastName: oldValue('lastName'),
  username: oldValue('username'),
})

function handleForm() {
  if (form.processing) return
  form.put(`${basePath}/${props.data.id}`)
}
</script>
