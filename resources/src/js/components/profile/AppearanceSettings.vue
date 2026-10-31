<script setup lang="ts">
import { appearance, editAppearance, saveAppearance, reloadAppearance } from '@/appearance/state'
import type { Preferences } from '@/appearance/state'
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
const groups = [
  {
    key: 'theme',
    label: 'Тема',
    options: [
      ['system', 'Системная'],
      ['light', 'Светлая'],
      ['dark', 'Тёмная'],
    ],
  },
  {
    key: 'locale',
    label: 'Язык',
    options: [
      ['ru', 'Русский'],
      ['en', 'English'],
    ],
  },
  {
    key: 'density',
    label: 'Плотность',
    options: [
      ['default', 'По умолчанию'],
      ['compact', 'Компактная'],
      ['comfortable', 'Комфортная'],
    ],
  },
] as const
function change(key: keyof Preferences, event: Event) {
  editAppearance({ [key]: (event.target as HTMLSelectElement).value })
}
</script>
<template>
  <section
    aria-labelledby="appearance-title"
    class="space-y-6 rounded-xl border border-border-primary bg-card p-6"
  >
    <div>
      <h2 id="appearance-title" class="text-xl font-semibold">{{ t('Интерфейс') }}</h2>
      <p class="mt-2 text-sm text-text-secondary">
        {{ t('Изменения сохраняются автоматически для вашей учётной записи.') }}
      </p>
    </div>
    <div
      v-for="group in groups"
      :key="group.key"
      class="flex max-w-md flex-col gap-2 text-sm font-medium"
    >
      <label :for="'appearance-' + group.key">{{ t(group.label) }}</label>
      <select
        :id="'appearance-' + group.key"
        :value="appearance.draft[group.key]"
        :disabled="appearance.readOnly || appearance.resolving"
        class="appearance-select rounded-lg border border-border-primary bg-card px-3 text-text-primary"
        @change="change(group.key, $event)"
      >
        <option v-for="[value, label] in group.options" :key="value" :value="value">
          {{ t(label) }}
        </option>
      </select>
    </div>
    <p v-if="appearance.readOnly" role="status">{{ t('Изменения недоступны в этом сеансе.') }}</p>
    <p v-else-if="appearance.status === 'saving'" role="status">{{ t('Сохраняем...') }}</p>
    <div v-else-if="appearance.status === 'error'" role="alert" class="space-y-3">
      <p>{{ t('Не удалось сохранить. Ваши изменения остаются на странице.') }}</p>
      <div class="flex flex-wrap gap-2">
        <AppButton @click="saveAppearance">{{ t('Повторить') }}</AppButton
        ><AppButton variant="secondary" @click="reloadAppearance(false)">{{
          t('Загрузить сохранённое')
        }}</AppButton>
      </div>
    </div>
    <div v-else-if="appearance.status === 'conflict'" role="alert" class="space-y-3">
      <p>{{ t('Настройки изменены в другом окне. Выберите, какие изменения сохранить.') }}</p>
      <div class="flex flex-wrap gap-2">
        <AppButton @click="reloadAppearance(true)">{{
          t('Применить мои изменения к новой версии')
        }}</AppButton
        ><AppButton variant="secondary" @click="reloadAppearance(false)">{{
          t('Загрузить сохранённое')
        }}</AppButton>
      </div>
    </div>
    <p v-else role="status" class="text-sm text-text-secondary">{{ t('Сохранено') }}</p>
  </section>
</template>
