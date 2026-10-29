<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import { t } from '@/i18n'
import AdminForm from './AdminForm.vue'
import FormButton from './FormButton.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Card, CardContent, CardFooter } from '@/components/ui/card'

defineProps<{
  form: { errors: Record<string, string>; processing: boolean; isDirty?: boolean }
  fields: string[]
  returnHref: string
  label: string
}>()
const emit = defineEmits<{ submit: [event: Event] }>()
</script>

<template>
  <div :inert="form.processing || undefined">
    <PageActionBar
      :back-href="returnHref"
      :back-text="t('К списку')"
      :label="label"
      :use-history="false"
    />
  </div>
  <Card class="overflow-hidden">
    <AdminForm :form="form" :fields="fields" warn-unsaved @submit="emit('submit', $event)">
      <CardContent class="pt-6 space-y-6"><slot /></CardContent>
      <template #actions>
        <CardFooter
          class="bg-surface border-t border-border-primary flex flex-wrap justify-end gap-3 py-3 px-6"
          :inert="form.processing || undefined"
        >
          <slot name="actions">
            <AppButton as-child variant="secondary" :disabled="form.processing">
              <Link :href="returnHref">{{ t('Отмена') }}</Link>
            </AppButton>
            <FormButton
              type="submit"
              variant="primary"
              :loading="form.processing"
              :loading-text="t('Сохранение...')"
              >{{ t('Сохранить') }}</FormButton
            >
          </slot>
        </CardFooter>
      </template>
    </AdminForm>
  </Card>
</template>
