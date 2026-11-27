<template>
  <div
    v-if="filterableColumns.length"
    data-ui="datagrid"
    data-part="filters"
    class="min-w-0 max-w-full"
    :class="{ 'w-full sm:basis-full': expanded && !inline }"
  >
    <div ref="summary" class="flex flex-wrap items-center gap-2" :class="{ 'sm:hidden': inline }">
      <AppButton
        variant="outline"
        size="sm"
        :aria-expanded="expanded"
        :aria-controls="panelId"
        @click="expanded = !expanded"
      >
        <SlidersHorizontal aria-hidden="true" />
        {{
          activeFilters.length
            ? t('Фильтры ({count})', undefined, { count: activeFilters.length })
            : t('Фильтры')
        }}
      </AppButton>
      <template v-if="!expanded">
        <AppButton
          v-for="filter in activeFilters"
          :key="filter.key"
          variant="secondary"
          size="sm"
          rounded
          class="min-w-0 max-w-full gap-1.5"
          :aria-label="t('Снять фильтр {filter}', undefined, { filter: filter.label })"
          :title="filter.label"
          @click="removeFilter(filter.key)"
        >
          <span class="truncate">{{ filter.label }}</span>
          <X aria-hidden="true" class="size-3.5" />
        </AppButton>
      </template>
    </div>

    <div
      :id="panelId"
      data-part="filter-panel"
      :class="[
        inline ? 'items-end gap-2' : 'gap-3 sm:grid-cols-2 lg:grid-cols-3',
        inline ? (expanded ? 'flex' : 'hidden sm:flex') : expanded ? 'grid' : 'hidden',
        inline ? 'mt-2 sm:mt-0 sm:items-center' : 'mt-2',
      ]"
    >
      <div
        v-for="(column, index) in displayColumns"
        :key="column.key"
        :class="inline ? 'min-w-0 flex-1 sm:w-44' : 'min-w-0'"
      >
        <label
          :for="`${panelId}-field-${index}`"
          class="mb-1 block text-sm font-medium text-text-primary"
          :class="{ 'sm:sr-only': inline }"
        >
          {{ t(column.label) }}
        </label>

        <AppSelect
          v-if="usesSelect(column)"
          :id="`${panelId}-field-${index}`"
          :model-value="getSelectValue(column)"
          :placeholder="
            column.placeholder ? t(column.placeholder) : t(inline ? column.label : 'Не выбрано')
          "
          :options="column.options || []"
          @update:modelValue="handleFilterChange(column.key, $event)"
        />

        <AppInput
          v-else
          :id="`${panelId}-field-${index}`"
          :type="column.type === 'number' ? 'number' : 'text'"
          :inputmode="column.type === 'number' ? 'numeric' : undefined"
          :model-value="getInputValue(column.key)"
          :placeholder="column.placeholder ? t(column.placeholder) : t(column.label)"
          @update:modelValue="handleFilterChange(column.key, $event)"
        />
      </div>

      <AppButton
        v-if="activeFilters.length"
        :variant="inline ? 'ghost' : 'outline'"
        :size="inline ? 'icon' : 'sm'"
        :class="inline ? 'shrink-0' : 'justify-self-start self-end'"
        :title="inline ? t('Очистить все') : undefined"
        @click="clearFilters"
      >
        <X v-if="inline" aria-hidden="true" />
        <span :class="{ 'sr-only': inline }">{{ t('Очистить все') }}</span>
      </AppButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { SlidersHorizontal, X } from 'lucide-vue-next'
import type { AcceptableValue } from 'reka-ui'
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import { dataGridOption, isActiveDataGridFilter, type Column } from '@/composables/useDataGrid'

const props = defineProps<{
  filterableColumns: Column[]
  filters: Record<string, unknown>
}>()

const emit = defineEmits<{
  'filter-change': [filters: Record<string, unknown>]
}>()

const panelId = `grid-filters-${useId()}`
const summary = ref<HTMLElement | null>(null)
const expanded = ref(false)
const inline = computed(() => props.filterableColumns.length === 1)
const usesSelect = (column: Column) =>
  column.type === 'select' || (column.type === 'badge' && Boolean(column.options?.length))
const displayColumns = computed(() =>
  props.filterableColumns.map((column) =>
    usesSelect(column) && column.options
      ? {
          ...column,
          options: column.options.map((option) => ({
            ...option,
            label: option.label ? t(option.label) : String(option.value),
          })),
        }
      : column,
  ),
)

const handleFilterChange = (key: string, value: AcceptableValue): void => {
  emit('filter-change', { [key]: value })
}

const getFilterValue = (key: string): unknown => {
  // Track the key before checking ownership so newly added filters stay reactive.
  const value = props.filters[key]
  return Object.hasOwn(props.filters, key) ? value : undefined
}

const getSelectValue = (column: Column): AcceptableValue | undefined => {
  const value = getFilterValue(column.key)
  if (!isActiveDataGridFilter(value)) return undefined
  return dataGridOption(column, value)?.value ?? (value as AcceptableValue)
}

const activeFilters = computed(() =>
  displayColumns.value.flatMap((column) => {
    const value = getFilterValue(column.key)
    if (!isActiveDataGridFilter(value)) return []
    const option = usesSelect(column) ? dataGridOption(column, value) : undefined
    let display = option?.label ?? String(value)
    if (typeof value === 'boolean' || column.type === 'boolean') {
      if (display === 'true') display = t('Да')
      if (display === 'false') display = t('Нет')
    }
    return [{ key: column.key, label: `${t(column.label)}: ${display}` }]
  }),
)

function removeFilter(key: string) {
  summary.value?.querySelector<HTMLButtonElement>('button[aria-controls]')?.focus()
  handleFilterChange(key, '')
}

const getInputValue = (key: string): string | number => {
  const value = getFilterValue(key)
  if (value === null || value === undefined) return ''
  if (typeof value === 'string' || typeof value === 'number') return value
  return String(value)
}

const clearFilters = (): void => {
  emit('filter-change', Object.fromEntries(props.filterableColumns.map(({ key }) => [key, ''])))
}
</script>
