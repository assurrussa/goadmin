<template>
  <Card class="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 px-5 py-4">
    <div class="mb-4 sm:mb-0">
      <h1 class="text-2xl font-bold text-text-primary">
        {{ props.meta?.title || 'Данные' }}
      </h1>
      <p v-if="props.meta?.description" class="mt-1 text-sm text-text-secondary">
        {{ props.meta?.description }}
      </p>
    </div>

    <div class="flex flex-col sm:flex-row sm:items-center gap-4">
      <!-- Поиск -->
      <div v-if="config.behaviour?.searchable" class="relative">
        <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
          <Search class="h-4 w-4 text-text-tertiary" />
        </div>
        <AppInput
          :model-value="searchQuery"
          @update:modelValue="$emit('search', String($event))"
          placeholder="Поиск..."
          class="pl-9"
        />
      </div>

      <!-- Дополнительные действия -->
      <div class="flex items-center gap-2">
        <!-- Кнопка обновления -->
        <AppButton
          v-if="config.behaviour?.refreshable"
          variant="outline"
          size="sm"
          @click="$emit('refresh')"
        >
          <RefreshCcw class="-ml-1 mr-2 h-4 w-4" />
          Обновить
        </AppButton>

        <!-- Кнопка экспорта -->
        <AppButton
          v-if="config.behaviour?.exportable"
          variant="outline"
          size="sm"
          @click="$emit('export')"
        >
          <Download class="-ml-1 mr-2 h-4 w-4" />
          Экспорт
        </AppButton>

        <!-- Кнопка создания -->
        <AppButton v-if="config.behaviour?.creatable" @click="$emit('create')" size="sm">
          <Plus class="-ml-1 mr-2 h-4 w-4" />
          {{ config.ui?.createButtonText || 'Создать' }}
        </AppButton>
      </div>
    </div>
  </Card>
</template>

<script setup lang="ts">
import type { Config, Meta } from '@/composables/useDataGrid.ts'
import { Card } from '@/components/ui/card'
import { Download, Plus, RefreshCcw, Search } from 'lucide-vue-next'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'

// Props interface
interface Props {
  meta: Meta | null
  config: Config
  searchQuery: string
}

// Define props with defaults
const props = withDefaults(defineProps<Props>(), {
  config: () => ({}),
  searchQuery: '',
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
