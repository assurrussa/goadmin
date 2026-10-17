<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" :label="t('Редактирование профиля')" :use-history="false" />

  <AdminForm
    :form="form"
    :fields="['name', 'lastName', 'username']"
    warn-unsaved
    @submit="handleForm"
  >
    <Card class="overflow-hidden">
      <CardContent class="pt-6">
        <div class="space-y-6">
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
        </div>
      </CardContent>
    </Card>

    <template v-slot:actions>
      <CardFooter
        class="bg-surface border-t border-border-primary flex justify-end gap-3 py-3 px-6"
      >
        <AppButton as-child variant="secondary">
          <Link :href="basePath">{{ t('Отмена') }}</Link>
        </AppButton>
        <FormButton
          type="submit"
          :loading="form.processing"
          :loading-text="t('Сохранение...')"
          variant="primary"
        >
          {{ t('Сохранить') }}
        </FormButton>
      </CardFooter>
    </template>
  </AdminForm>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { Link, useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Card, CardContent, CardFooter } from '@/components/ui/card'

const basePath = '/auth/profile'
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
  lastName: oldValue('lastName'),
  username: oldValue('username'),
})

function handleForm() {
  if (form.processing) return
  form.post(`${basePath}/edit`, { preserveState: 'errors' })
}
</script>
