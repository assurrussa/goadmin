<template>
  <section
    data-ui="datagrid"
    data-part="workflow"
    :aria-label="t('Представления и действия')"
    class="space-y-3 rounded-lg border border-border-primary bg-card p-4 text-text-primary"
  >
    <div class="flex flex-wrap items-end gap-2">
      <label class="min-w-0 max-w-full text-sm"
        >{{ t('Представление') }}
        <select
          :value="activeViewId"
          :disabled="!ready || busy"
          class="block max-w-full rounded border border-border-primary bg-card p-2"
          @change="applyView(($event.target as HTMLSelectElement).value)"
        >
          <option value="">{{ t('Текущие условия') }}</option>
          <option v-for="view in preferences.views" :key="view.id" :value="view.id">
            {{ view.name }}
          </option>
        </select>
      </label>
      <label class="min-w-0 text-sm"
        >{{ t('Название представления')
        }}<AppInput v-model="name" :maxlength="120" :disabled="!ready || busy"
      /></label>
      <AppButton
        variant="outline"
        :disabled="!ready || busy || conflict || !name.trim()"
        @click="saveView(name)"
        >{{ t('Сохранить как новое') }}</AppButton
      >
      <AppButton
        variant="outline"
        :disabled="!activeViewId || busy || conflict || !name.trim()"
        @click="saveView(name, true)"
        >{{ t('Сохранить / переименовать') }}</AppButton
      >
      <AppButton
        variant="outline"
        :disabled="!activeViewId || busy || conflict"
        @click="deleteView"
        >{{ t('Удалить представление') }}</AppButton
      >
      <AppButton variant="outline" :disabled="!ready || busy || conflict" @click="setDefault"
        >{{ t('По умолчанию')
        }}{{
          preferences.defaultViewId && preferences.defaultViewId === activeViewId ? ' ✓' : ''
        }}</AppButton
      >
    </div>
    <details>
      <summary class="cursor-pointer text-sm">{{ t('Столбцы') }}</summary>
      <div class="my-3 flex flex-wrap gap-3">
        <label
          v-for="column in availableColumns"
          :key="column.key"
          class="flex items-center gap-2 text-sm"
        >
          <input
            type="checkbox"
            :checked="columns.includes(column.key)"
            :disabled="!ready || busy || (columns.length === 1 && columns.includes(column.key))"
            @change="toggleColumn(column.key)"
          />{{ t(column.label) }}
        </label>
      </div>
      <ol class="space-y-1">
        <li v-for="(key, index) in columns" :key="key" class="flex items-center gap-2 text-sm">
          <span>{{ t(availableColumns.find((col) => col.key === key)?.label || key) }}</span>
          <AppButton
            size="sm"
            variant="outline"
            :aria-label="t('Переместить вверх')"
            :disabled="index === 0 || busy || !ready"
            @click="moveColumn(index, -1)"
            >↑</AppButton
          >
          <AppButton
            size="sm"
            variant="outline"
            :aria-label="t('Переместить вниз')"
            :disabled="index === columns.length - 1 || busy || !ready"
            @click="moveColumn(index, 1)"
            >↓</AppButton
          >
        </li>
      </ol>
      <div class="mt-3 flex flex-wrap gap-2">
        <AppButton
          variant="outline"
          :disabled="!ready || busy || conflict"
          @click="savePreferences"
          >{{ t('Сохранить столбцы') }}</AppButton
        >
        <AppButton
          variant="outline"
          :disabled="!ready || busy"
          @click="preferences.columns = availableColumns.map((col) => col.key)"
          >{{ t('Сбросить столбцы') }}</AppButton
        >
      </div>
    </details>
    <div class="flex flex-wrap items-center gap-2">
      <span role="status" class="text-sm">{{
        t('Выбрано: {count} из 100', undefined, { count: selectedCount })
      }}</span>
      <AppButton
        variant="outline"
        :disabled="!selectedCount || busy"
        @click="$emit('clear-selection')"
        >{{ t('Снять выделение') }}</AppButton
      >
      <AppButton
        variant="outline"
        :disabled="!selectedCount || busy || loading"
        @click="exportCSV('selected')"
        >{{ t('CSV выбранных') }}</AppButton
      >
      <AppButton variant="outline" :disabled="busy || loading" @click="exportCSV('filtered')">{{
        t('CSV по фильтрам')
      }}</AppButton>
      <AppButton
        v-for="action in workflow?.bulkActions"
        :key="action.key"
        variant="outline"
        :disabled="!selectedCount || busy || loading"
        @click="pendingAction = action.key"
        >{{ t(action.label) }}</AppButton
      >
    </div>
    <p v-if="viewDirty" role="status" data-part="view-dirty" class="text-sm text-text-secondary">
      {{ t('Представление изменено. Сохраните изменения.') }}
    </p>
    <p v-if="error" role="alert" class="text-error">{{ t(error) }}</p>
    <AppButton
      v-if="conflict || (!ready && error)"
      variant="outline"
      :disabled="busy"
      @click="loadPreferences"
      >{{ t('Загрузить серверные настройки') }}</AppButton
    >
    <p v-if="notice" role="status">{{ t(notice) }}</p>
    <div
      v-if="results.length"
      class="max-h-80 overflow-auto"
      role="region"
      :aria-label="t('Результаты операции')"
      tabindex="0"
    >
      <p>{{ t('Результаты операции') }}</p>
      <p class="text-sm">
        {{ t('Неизвестный результат не повторяется автоматически. Проверьте состояние записи.') }}
      </p>
      <ul>
        <li v-for="result in results" :key="result.id" class="break-all text-sm">
          {{ result.id }}: {{ t(`bulk.${result.status}`)
          }}<span v-if="result.code"> ({{ result.code }})</span
          ><span v-if="result.relatedId"> → {{ result.relatedId }}</span>
        </li>
      </ul>
    </div>
    <Dialog
      :open="!!pendingAction"
      @update:open="
        (value) => {
          if (!value) pendingAction = ''
        }
      "
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('Подтвердите операцию') }}</DialogTitle>
          <DialogDescription>{{
            t(
              'Операция «{action}» затронет {count} выбранных записей на всех страницах.',
              undefined,
              {
                action: t(
                  workflow?.bulkActions?.find((action) => action.key === pendingAction)?.label ||
                    '',
                ),
                count: selectedCount,
              },
            )
          }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <AppButton variant="outline" @click="pendingAction = ''">{{ t('Отмена') }}</AppButton>
          <AppButton :disabled="busy || loading || !selectedCount" @click="executeBulk">{{
            t('Подтвердить')
          }}</AppButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>
<script setup lang="ts">
import { t } from '@/i18n'
import type { useDataGridWorkflow } from '@/composables/useDataGridWorkflow'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
const props = defineProps<{
  model: ReturnType<typeof useDataGridWorkflow>
  selectedCount: number
  loading: boolean
}>()
defineEmits<{ 'clear-selection': [] }>()
const {
  workflow,
  preferences,
  activeViewId,
  viewName: name,
  viewDirty,
  availableColumns,
  columns,
  error,
  notice,
  ready,
  busy,
  conflict,
  results,
  pendingAction,
  applyView,
  savePreferences,
  saveView,
  deleteView,
  setDefault,
  toggleColumn,
  moveColumn,
  exportCSV,
  executeBulk,
  loadPreferences,
} = props.model
</script>
