<script setup lang="ts">
import { computed, nextTick, reactive, ref, shallowRef, useId, watch } from 'vue'
import { getLocalTimeZone, parseDate, today, type DateValue } from '@internationalized/date'
import {
  CalendarRoot,
  CalendarHeader,
  CalendarHeading,
  CalendarPrev,
  CalendarNext,
  CalendarGrid,
  CalendarGridHead,
  CalendarGridBody,
  CalendarGridRow,
  CalendarHeadCell,
  CalendarCell,
  CalendarCellTrigger,
  PopoverRoot,
  PopoverTrigger,
  PopoverPortal,
  PopoverContent,
  PopoverClose,
} from 'reka-ui'
import { CalendarDays, ChevronLeft, ChevronRight, X } from 'lucide-vue-next'
import { locale, t } from '@/i18n'
import { dateParts, dateValue, type DatePart } from './dateValue'

const props = withDefaults(
  defineProps<{
    modelValue: string | null
    id?: string
    name?: string
    label?: string
    required?: boolean
    disabled?: boolean
    readonly?: boolean
    withTime?: boolean
    error?: string
    describedby?: string
  }>(),
  { withTime: false },
)
const emit = defineEmits<{
  'update:modelValue': [value: string | null]
  'update:valid': [valid: boolean]
}>()
const generatedId = useId()
const controlId = computed(() => props.id ?? generatedId)
const language = computed(() => (locale.value === 'en' ? 'en-GB' : 'ru-RU'))
const separator = computed(() => (locale.value === 'en' ? '/' : '.'))
const parts = reactive(dateParts(props.modelValue ?? '', props.withTime))
const open = ref(false)
const dirty = ref(false)
const minimumDate = parseDate('0001-01-01')
const maximumDate = parseDate('9999-12-31')
const inputs = new Map<DatePart, HTMLInputElement>()
const keys = computed<DatePart[]>(() =>
  props.withTime ? ['day', 'month', 'year', 'hour', 'minute'] : ['day', 'month', 'year'],
)
const labels: Record<DatePart, string> = {
  day: 'День',
  month: 'Месяц',
  year: 'Год',
  hour: 'Часы',
  minute: 'Минуты',
}
const placeholders = computed<Record<DatePart, string>>(() =>
  locale.value === 'ru'
    ? { day: 'ДД', month: 'ММ', year: 'ГГГГ', hour: 'ЧЧ', minute: 'ММ' }
    : { day: 'DD', month: 'MM', year: 'YYYY', hour: 'HH', minute: 'mm' },
)
const value = computed(() => dateValue(parts, props.withTime))
const valid = computed(() => value.value !== null && (!props.required || value.value !== ''))
const message = computed(() =>
  value.value === ''
    ? t('Введите дату.')
    : t(
        props.withTime
          ? 'Введите полную корректную дату и время.'
          : 'Введите полную корректную дату.',
      ),
)
const draftError = computed(() => (dirty.value && !valid.value ? message.value : ''))
const description = computed(
  () =>
    [props.describedby, draftError.value ? `${controlId.value}-draft-error` : '']
      .filter(Boolean)
      .join(' ') || undefined,
)
const selected = computed(() => {
  if (!value.value) return undefined
  return parseDate(value.value.slice(0, 10))
})
const placeholder = shallowRef<DateValue>(selected.value ?? today(getLocalTimeZone()))
let emitted: string | null | undefined
watch(
  () => props.modelValue,
  (incoming) => {
    if (incoming === emitted) {
      emitted = undefined
      return
    }
    Object.assign(parts, dateParts(incoming ?? '', props.withTime))
    dirty.value = false
  },
)
watch(selected, (date) => {
  if (date) placeholder.value = date
})
watch(
  [valid, message, () => props.disabled, () => props.readonly],
  () => {
    emit('update:valid', valid.value)
    for (const input of inputs.values())
      input.setCustomValidity(props.disabled || props.readonly || valid.value ? '' : message.value)
  },
  { immediate: true, flush: 'post' },
)
function bindInput(part: DatePart, element: unknown) {
  if (element instanceof HTMLInputElement) {
    inputs.set(part, element)
    element.setCustomValidity(props.disabled || props.readonly || valid.value ? '' : message.value)
  } else inputs.delete(part)
}
function publish() {
  dirty.value = true
  // Clear the wire value immediately for partial edits; a previous complete date is never retained.
  emitted = value.value === null ? '' : value.value
  emit('update:modelValue', emitted)
  emit('update:valid', valid.value)
}
function input(part: DatePart, event: Event) {
  if (props.disabled || props.readonly) return
  parts[part] = (event.target as HTMLInputElement).value
  publish()
}
function blur(part: DatePart) {
  if (!props.disabled && !props.readonly && part !== 'year' && /^\d$/.test(parts[part]))
    parts[part] = parts[part].padStart(2, '0')
}
function keydown(part: DatePart, event: KeyboardEvent) {
  if (props.disabled || props.readonly || event.altKey || event.ctrlKey || event.metaKey) return
  const index = keys.value.indexOf(part)
  const target = event.target as HTMLInputElement
  if (
    (event.key === 'ArrowRight' && target.selectionStart === target.value.length) ||
    (event.key === 'ArrowLeft' && target.selectionStart === 0)
  ) {
    event.preventDefault()
    inputs.get(keys.value[index + (event.key === 'ArrowRight' ? 1 : -1)]!)?.focus()
  }
  if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    event.preventDefault()
    const limits: Record<DatePart, [number, number]> = {
      day: [1, 31],
      month: [1, 12],
      year: [1, 9999],
      hour: [0, 23],
      minute: [0, 59],
    }
    const [min, max] = limits[part]
    const previous = Number(parts[part] || min)
    parts[part] = String(
      Math.max(min, Math.min(max, previous + (event.key === 'ArrowUp' ? 1 : -1))),
    ).padStart(part === 'year' ? 4 : 2, '0')
    publish()
  }
}
function choose(date: DateValue | undefined) {
  if (!date || props.disabled || props.readonly) return
  if (date.compare(minimumDate) < 0 || date.compare(maximumDate) > 0) return
  Object.assign(parts, {
    day: String(date.day).padStart(2, '0'),
    month: String(date.month).padStart(2, '0'),
    year: String(date.year).padStart(4, '0'),
  })
  // Calendar selection preserves the local wall-clock time, including midnight.
  publish()
  open.value = false
}
function clear() {
  if (props.disabled || props.readonly) return
  Object.assign(parts, dateParts('', props.withTime))
  emitted = props.modelValue === null ? null : ''
  emit('update:modelValue', emitted)
  dirty.value = true
  void nextTick(() => inputs.get('day')?.focus())
}
watch(
  () => props.disabled || props.readonly,
  (blocked) => {
    if (blocked) open.value = false
  },
)
</script>

<template>
  <div class="date-control" :data-invalid="!!error || !!draftError || undefined">
    <div
      role="group"
      :aria-label="label"
      :aria-describedby="description"
      :aria-invalid="!!error || !!draftError"
      class="date-control-field"
    >
      <template v-for="(part, index) in keys" :key="part">
        <span v-if="index" aria-hidden="true" class="date-control-separator">{{
          part === 'hour' ? ' ' : part === 'minute' ? ':' : separator
        }}</span>
        <input
          :id="part === 'day' ? controlId : `${controlId}-${part}`"
          :ref="(element) => bindInput(part, element)"
          :data-date-part="part"
          :data-field-name="part === 'day' ? name : undefined"
          :value="parts[part]"
          type="text"
          inputmode="numeric"
          autocomplete="off"
          :maxlength="part === 'year' ? 4 : 2"
          :placeholder="placeholders[part]"
          :aria-label="`${label ? label + ': ' : ''}${t(labels[part])}`"
          :aria-invalid="!!error || !!draftError"
          :aria-describedby="description"
          :required="required"
          :disabled="disabled"
          :readonly="readonly"
          :class="['date-control-segment', { 'date-control-year': part === 'year' }]"
          @input="input(part, $event)"
          @keydown="keydown(part, $event)"
          @blur="blur(part)"
          @invalid="dirty = true"
        />
      </template>
      <input v-if="name" type="hidden" :name="name" :value="value ?? ''" :disabled="disabled" />
      <PopoverRoot v-model:open="open">
        <PopoverTrigger
          class="date-control-button"
          :disabled="disabled || readonly"
          :aria-label="t('Открыть календарь')"
        >
          <CalendarDays class="h-4 w-4" aria-hidden="true" />
        </PopoverTrigger>
        <PopoverPortal>
          <PopoverContent
            class="date-control-popover"
            align="end"
            :side-offset="6"
            :aria-label="label || t('Открыть календарь')"
          >
            <CalendarRoot
              v-slot="{ grid, weekDays }"
              v-model:placeholder="placeholder"
              :model-value="selected"
              :min-value="minimumDate"
              :max-value="maximumDate"
              :locale="language"
              :week-starts-on="1"
              weekday-format="short"
              :prevent-deselect="true"
              initial-focus
              @update:model-value="choose"
            >
              <CalendarHeader class="date-control-header">
                <CalendarPrev class="date-control-button" :aria-label="t('Предыдущий месяц')"
                  ><ChevronLeft class="h-4 w-4" aria-hidden="true"
                /></CalendarPrev>
                <CalendarHeading class="font-medium" />
                <CalendarNext class="date-control-button" :aria-label="t('Следующий месяц')"
                  ><ChevronRight class="h-4 w-4" aria-hidden="true"
                /></CalendarNext>
              </CalendarHeader>
              <CalendarGrid
                v-for="month in grid"
                :key="month.value.toString()"
                class="date-control-grid"
              >
                <CalendarGridHead
                  ><CalendarGridRow
                    ><CalendarHeadCell
                      v-for="day in weekDays"
                      :key="day"
                      class="date-control-weekday"
                      >{{ day }}</CalendarHeadCell
                    ></CalendarGridRow
                  ></CalendarGridHead
                >
                <CalendarGridBody>
                  <CalendarGridRow v-for="(week, index) in month.rows" :key="index">
                    <CalendarCell v-for="day in week" :key="day.toString()" :date="day">
                      <CalendarCellTrigger
                        as="button"
                        type="button"
                        :day="day"
                        :month="month.value"
                        class="date-control-day"
                      />
                    </CalendarCell>
                  </CalendarGridRow>
                </CalendarGridBody>
              </CalendarGrid>
            </CalendarRoot>
            <PopoverClose class="date-control-close">{{ t('Закрыть календарь') }}</PopoverClose>
          </PopoverContent>
        </PopoverPortal>
      </PopoverRoot>
      <button
        v-if="!required"
        type="button"
        class="date-control-button"
        :disabled="disabled || readonly || keys.every((part) => !parts[part])"
        :aria-label="t('Очистить дату')"
        @click="clear"
      >
        <X class="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
    <p v-if="draftError" :id="`${controlId}-draft-error`" class="text-sm text-error" role="alert">
      {{ draftError }}
    </p>
  </div>
</template>

<style scoped>
.date-control-field {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  width: fit-content;
  max-width: 100%;
  padding: 2px 4px;
  border: 1px solid var(--color-border-primary);
  border-radius: var(--goadmin-control-radius, 4px);
  background: var(--color-card);
  color: var(--color-text-primary);
}
.date-control-field:focus-within {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.date-control-segment {
  width: 44px;
  height: 44px;
  border: 0;
  border-radius: 2px;
  background: transparent;
  color: inherit;
  text-align: center;
  font-variant-numeric: tabular-nums;
}
.date-control-year {
  width: 3.25rem;
}
.date-control-segment:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: -2px;
}
.date-control-segment:disabled {
  opacity: 0.55;
}
.date-control-separator {
  white-space: pre;
  color: var(--color-text-secondary);
}
.date-control-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  border-radius: var(--goadmin-control-radius, 4px);
  cursor: pointer;
}
.date-control-button:hover:not(:disabled),
.date-control-day:hover:not(:disabled),
.date-control-close:hover {
  background: var(--color-surface);
}
.date-control-button:focus-visible,
.date-control-day:focus-visible,
.date-control-close:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 2px;
}
.date-control-button:disabled {
  opacity: 0.45;
  cursor: default;
}
.date-control[data-invalid] .date-control-field {
  border-color: var(--color-error);
}
/* Reka teleports this root without the caller's scoped attribute. */
:global(.date-control-popover) {
  z-index: 60;
  width: max-content;
  max-width: calc(100vw - 2px);
  padding: 4px;
  border: 1px solid var(--color-border-primary);
  border-radius: var(--goadmin-control-radius, 4px);
  background: var(--color-card);
  color: var(--color-text-primary);
  box-shadow: 0 8px 24px rgb(0 0 0 / 0.18);
}
.date-control-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.date-control-grid {
  border-collapse: collapse;
  table-layout: fixed;
}
.date-control-weekday {
  height: 36px;
  font-size: 0.75rem;
  font-weight: normal;
  color: var(--color-text-secondary);
}
.date-control-day {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  cursor: pointer;
  border-radius: var(--goadmin-control-radius, 4px);
  font-variant-numeric: tabular-nums;
}
.date-control-day[data-outside-view] {
  color: var(--color-text-secondary);
}
.date-control-day[data-today] {
  box-shadow: inset 0 0 0 1px var(--color-primary);
}
.date-control-day[data-selected],
.date-control-day[data-selected]:hover {
  background: var(--color-primary);
  color: var(--color-primary-foreground, white);
  opacity: 1;
}
.date-control-close {
  display: block;
  margin: 4px 0 0 auto;
  min-height: 44px;
  padding: 4px 8px;
  border-radius: var(--goadmin-control-radius, 4px);
  cursor: pointer;
}
</style>
