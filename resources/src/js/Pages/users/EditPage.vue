<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" label="Редактирование" />

  <AdminForm
    :form="form"
    :fields="['username', 'name', 'lastName', 'email']"
    warn-unsaved
    @submit="handleForm"
  >
    <Card class="overflow-hidden">
      <CardContent class="pt-6">
        <div class="space-y-6">
          <FormInput
            v-model="form.name"
            name="name"
            type="text"
            label="Имя"
            required
            :error="form.errors.name"
          />
          <FormInput
            v-model="form.lastName"
            name="lastName"
            type="text"
            label="Фамилия"
            :error="form.errors.lastName"
          />
          <FormInput
            v-model="form.email"
            name="email"
            type="email"
            label="E-mail"
            required
            :error="form.errors.email"
            :readonly="!authmail"
          />
          <FormInput
            v-model="form.username"
            name="username"
            type="text"
            label="Nickname"
            :error="form.errors.username"
          />
        </div>
      </CardContent>

      <CardFooter class="bg-surface border-t border-border-primary flex justify-end py-3 px-6">
        <FormButton
          type="submit"
          :loading="form.processing"
          loading-text="Сохранение..."
          variant="primary"
          >Сохранить</FormButton
        >
      </CardFooter>
    </Card>
  </AdminForm>
</template>

<script setup lang="ts">
import { useForm, usePage } from '@inertiajs/vue3'
import { watch } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'
import { Card, CardContent, CardFooter } from '@/components/ui/card'

const { authmail } = useAdminCapabilities()
const basePath = '/users'
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
