<script setup lang="ts">
defineOptions({ name: 'CrudField' })
import { computed, useId } from 'vue'
import { t } from '@/i18n'
import LocalizedDatePicker from '@/components/date/LocalizedDatePicker.vue'
import { resolveCrudField, crudValue, type CrudField, type CrudValue } from './index'
const props = defineProps<{
  field: CrudField
  modelValue: CrudValue
  error?: string
  disabled?: boolean
  readonly?: boolean
}>()
const emit = defineEmits<{
  'update:modelValue': [value: CrudValue]
  'update:valid': [valid: boolean]
}>()
const id = useId()
const component = computed(() => resolveCrudField(props.field.component))
function customChange(value: unknown) {
  if (!props.disabled && !props.readonly) emit('update:modelValue', crudValue(value))
}
function change(event: Event) {
  if (props.disabled || props.readonly) return
  const target = event.target as HTMLInputElement
  const value =
    props.field.type === 'boolean'
      ? target.checked
      : props.field.type === 'number'
        ? target.value === ''
          ? null
          : Number(target.value)
        : target.value
  emit('update:modelValue', value)
}
</script>
<template>
  <component
    v-if="field.type === 'custom' && component"
    :is="component"
    :field="field"
    :model-value="modelValue"
    :error="error"
    :disabled="!!disabled"
    :readonly="!!readonly"
    @update:model-value="customChange"
  />
  <div v-else class="space-y-2">
    <label
      v-if="field.type === 'boolean'"
      :for="id"
      class="flex min-h-11 items-center gap-3 text-sm font-medium"
    >
      <input
        :id="id"
        :name="field.key"
        type="checkbox"
        :checked="modelValue === true"
        :disabled="disabled || readonly"
        :aria-invalid="!!error"
        :aria-describedby="error ? `${id}-error` : undefined"
        class="h-5 w-5 rounded border-border-primary accent-primary"
        @change="change"
      />
      {{ t(field.label) }}
    </label>
    <label v-else :for="id" class="block text-sm font-medium"
      >{{ t(field.label) }}<span v-if="field.required"> *</span></label
    >
    <p v-if="field.type === 'custom'" role="alert">{{ t('Компонент поля недоступен.') }}</p>
    <textarea
      v-else-if="field.type === 'textarea'"
      :id="id"
      :name="field.key"
      :value="String(modelValue ?? '')"
      :disabled="disabled"
      :readonly="readonly"
      :required="field.required"
      :aria-invalid="!!error"
      :aria-describedby="error ? `${id}-error` : undefined"
      class="w-full rounded-lg border border-border-primary bg-card px-3 py-2 text-text-primary"
      @input="change"
    />
    <select
      v-else-if="field.type === 'select'"
      :id="id"
      :name="field.key"
      :value="modelValue"
      :disabled="disabled || readonly"
      :required="field.required"
      :aria-invalid="!!error"
      :aria-describedby="error ? `${id}-error` : undefined"
      class="appearance-select w-full rounded-lg border border-border-primary bg-card px-3 text-text-primary"
      @change="change"
    >
      <option v-if="!field.required" value="">—</option>
      <option v-for="option in field.options" :key="option.value" :value="option.value">
        {{ t(option.label) }}
      </option>
    </select>
    <input
      v-else-if="field.type !== 'boolean' && field.type !== 'date'"
      :id="id"
      :name="field.key"
      :type="field.type === 'number' ? 'number' : 'text'"
      :value="String(modelValue ?? '')"
      :step="field.type === 'number' ? 'any' : undefined"
      :disabled="disabled"
      :readonly="readonly"
      :required="field.required"
      :aria-invalid="!!error"
      :aria-describedby="error ? `${id}-error` : undefined"
      class="appearance-select w-full rounded-lg border border-border-primary bg-card px-3 text-text-primary"
      @input="change"
    />
    <LocalizedDatePicker
      v-else-if="field.type === 'date'"
      :id="id"
      :name="field.key"
      :label="t(field.label)"
      :model-value="modelValue === null ? null : String(modelValue ?? '')"
      :required="field.required"
      :disabled="disabled"
      :readonly="readonly"
      :error="error"
      :describedby="error ? `${id}-error` : undefined"
      @update:model-value="customChange"
      @update:valid="emit('update:valid', $event)"
    />
    <p v-if="error" :id="`${id}-error`" class="text-sm text-error">{{ t(error) }}</p>
  </div>
</template>
