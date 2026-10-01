<template>
  <Card v-if="filterableColumns.length > 0">
    <CardContent class="p-6">
      <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-medium text-text-primary">Фильтры</h3>
        <AppButton
          v-if="hasActiveFilters"
          @click="clearFilters"
          variant="ghost"
          size="sm"
          class="text-text-tertiary hover:text-text-secondary"
        >
          Очистить все
        </AppButton>
      </div>

      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
        <div v-for="column in filterableColumns" :key="column.key" class="flex flex-col">
          <label
            :for="`filter-${column.key}`"
            class="block text-sm font-medium text-text-primary mb-2"
          >
            {{ column.label }}
          </label>

          <!-- Select фильтр -->
          <AppSelect
            v-if="column.type === 'select'"
            :id="`filter-${column.key}`"
            :model-value="getSelectValue(column.key)"
            :placeholder="column.placeholder || 'Не выбрано'"
            :options="column.options || []"
            @update:modelValue="handleFilterChange(column.key, $event)"
          />

          <!-- Text фильтр -->
          <AppInput
            v-else
            :id="`filter-${column.key}`"
            :type="column.type === 'number' ? 'number' : 'text'"
            :inputmode="column.type === 'number' ? 'numeric' : undefined"
            :model-value="getInputValue(column.key)"
            @update:modelValue="handleFilterChange(column.key, $event)"
            :placeholder="column.placeholder || `Фильтр по ${column.label.toLowerCase()}`"
          />
        </div>
      </div>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { AcceptableValue } from 'reka-ui'
import { Card, CardContent } from '@/components/ui/card'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import type { Column } from '../../composables/useDataGrid.ts'

// Props interface
interface Props {
  filterableColumns: Column[]
  filters: Record<string, unknown>
}

// Define props with defaults
const props = withDefaults(defineProps<Props>(), {
  filterableColumns: () => [],
  filters: () => ({}),
})

// Define emits
const emit = defineEmits<{
  'filter-change': [filters: Record<string, unknown>]
}>()

const hasActiveFilters = computed((): boolean => {
  return Object.values(props.filters).some(
    (value) => value !== null && value !== undefined && value !== '',
  )
})

const handleFilterChange = (key: string, value: AcceptableValue): void => {
  emit('filter-change', { [key]: value })
}

const getSelectValue = (key: string): AcceptableValue | undefined => {
  const value = props.filters[key]
  if (value === '' || value === null || value === undefined) return undefined
  return value as AcceptableValue
}

const getInputValue = (key: string): string | number | null => {
  const value = props.filters[key]
  if (value === null || value === undefined) return ''
  if (typeof value === 'string' || typeof value === 'number') return value
  return String(value)
}

const clearFilters = (): void => {
  const clearedFilters: Record<string, string> = {}
  props.filterableColumns.forEach((column) => {
    clearedFilters[column.key] = ''
  })
  emit('filter-change', clearedFilters)
}
</script>
