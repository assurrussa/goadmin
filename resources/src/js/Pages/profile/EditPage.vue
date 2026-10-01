<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" label="Редактирование" />

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
            label="Имя"
            required
            :error="form.errors.name"
          />

          <!-- Last Name -->
          <FormInput
            v-model="form.lastName"
            name="lastName"
            type="text"
            label="Фамилия"
            :error="form.errors.lastName"
          />

          <!-- Username -->
          <FormInput
            v-model="form.username"
            name="username"
            type="text"
            label="Имя пользователя"
            :error="form.errors.username"
          />
        </div>
      </CardContent>
    </Card>

    <template v-slot:actions>
      <CardFooter class="bg-surface border-t border-border-primary flex justify-end py-3 px-6">
        <FormButton
          type="submit"
          :loading="form.processing"
          loading-text="Сохранение..."
          variant="primary"
        >
          Сохранить
        </FormButton>
      </CardFooter>
    </template>
  </AdminForm>
</template>

<script setup lang="ts">
import { useForm, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import FormInput from '@/components/form/FormInput.vue'
import FormButton from '@/components/form/FormButton.vue'
import AdminForm from '@/components/form/AdminForm.vue'
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
const oldData = (page.props.old as Record<string, string | null>) || {}

const form = useForm({
  name: oldData.name || props.data.name,
  lastName: oldData.lastName || props.data.lastName || null,
  username: oldData.username || props.data.username || null,
})

function handleForm() {
  form.post(`${basePath}/edit`)
}
</script>
