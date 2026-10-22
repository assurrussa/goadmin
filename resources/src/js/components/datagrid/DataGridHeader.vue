<template>
  <div
    data-ui="datagrid"
    data-part="header"
    class="flex flex-col gap-4 px-0 py-4 xl:flex-row xl:items-center xl:justify-between"
  >
    <div class="mb-4 sm:mb-0">
      <h1 class="text-2xl font-semibold tracking-tight text-text-primary">
        {{ (props.meta?.title ? t(props.meta.title) : '') || t('Данные') }}
      </h1>
      <p v-if="props.meta?.description" class="mt-1 text-sm text-text-secondary">
        {{ props.meta?.description }}
      </p>
    </div>

    <div class="flex min-w-0 flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
      <!-- Поиск -->
      <div v-if="config.behaviour?.searchable" class="relative">
        <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
          <Search class="h-4 w-4 text-text-tertiary" />
        </div>
        <AppInput
          :model-value="searchQuery"
          @update:modelValue="$emit('search', String($event))"
          :placeholder="t('Поиск...')"
          class="pl-9"
        />
      </div>

      <!-- Дополнительные действия -->
      <div class="flex flex-wrap items-center gap-2">
        <!-- Кнопка обновления -->
        <AppButton
          v-if="config.behaviour?.refreshable"
          variant="outline"
          size="sm"
          @click="$emit('refresh')"
        >
          <RefreshCcw class="-ml-1 mr-2 h-4 w-4" />
          {{ t('Обновить') }}
        </AppButton>

        <!-- Кнопка экспорта -->
        <AppButton
          v-if="config.behaviour?.exportable"
          :disabled="exportDisabled"
          variant="outline"
          size="sm"
          @click="$emit('export')"
        >
          <Download class="-ml-1 mr-2 h-4 w-4" />
          {{ t('Экспорт') }}
        </AppButton>

        <!-- Кнопка создания -->
        <AppButton v-if="config.behaviour?.creatable" @click="$emit('create')" size="sm">
          <Plus class="-ml-1 mr-2 h-4 w-4" />
          {{ config.ui?.createButtonText || t('Создать') }}
        </AppButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import type { Config, Meta } from '@/composables/useDataGrid.ts'
import { Download, Plus, RefreshCcw, Search } from 'lucide-vue-next'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'

// Props interface
interface Props {
  meta: Meta | null
  config: Config
  searchQuery: string
  exportDisabled?: boolean
}

// Define props with defaults
const props = withDefaults(defineProps<Props>(), {
  config: () => ({}),
  searchQuery: '',
  exportDisabled: false,
  title: '',
  description: '',
})

// Define emits
defineEmits<{
  search: [query: string]
  create: []
  refresh: []
  export: []
}>()
</script>
