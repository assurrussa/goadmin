<template>
  <div
    ref="header"
    data-ui="datagrid"
    data-part="header"
    tabindex="-1"
    class="flex flex-col gap-4 px-0 py-4 xl:flex-row xl:items-start xl:justify-between"
  >
    <div>
      <h1 class="text-2xl font-semibold tracking-tight text-text-primary">
        {{ (props.meta?.title ? t(props.meta.title) : '') || t('Данные') }}
      </h1>
      <p v-if="props.meta?.description" class="mt-1 text-sm text-text-secondary">
        {{ props.meta?.description }}
      </p>
    </div>

    <div
      ref="toolbar"
      class="flex min-w-0 flex-col gap-3 empty:hidden sm:flex-row sm:flex-wrap sm:items-center"
    >
      <!-- Поиск -->
      <div v-if="searchable" class="relative">
        <div class="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
          <Search class="h-4 w-4 text-text-tertiary" />
        </div>
        <AppInput
          :model-value="searchQuery"
          @update:modelValue="$emit('search', String($event))"
          :placeholder="t('Поиск...')"
          :aria-label="t('Поиск')"
          data-grid-search
          class="pl-9"
        />
      </div>

      <slot />

      <!-- Дополнительные действия -->
      <div
        v-if="
          config.behaviour?.refreshable ||
          config.behaviour?.exportable ||
          config.behaviour?.creatable
        "
        class="flex flex-wrap items-center gap-2"
      >
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
          {{ t(config.ui?.createButtonText || 'Создать') }}
        </AppButton>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'
import { computed, ref } from 'vue'

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

// Keep legacy explicit flags authoritative; current API responses advertise searchMode.
const searchable = computed(
  () =>
    props.config.behaviour?.searchable ??
    ['server', 'client'].includes(props.config.behaviour?.searchMode ?? ''),
)

const header = ref<HTMLElement | null>(null)
const toolbar = ref<HTMLElement | null>(null)
function focusControls(): void {
  if (!toolbar.value) return
  for (const control of toolbar.value.querySelectorAll<HTMLElement>(
    'input:not([disabled]), button:not([disabled])',
  )) {
    control.focus()
    if (control.ownerDocument.activeElement === control) return
  }
  header.value?.focus()
}
defineExpose({ focusControls })

// Define emits
defineEmits<{
  search: [query: string]
  create: []
  refresh: []
  export: []
}>()
</script>
