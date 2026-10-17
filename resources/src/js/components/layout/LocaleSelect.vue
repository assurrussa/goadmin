<script setup lang="ts">
import { useI18n } from '@/i18n'
const { t, locale, setLocale } = useI18n()
</script>
<template>
  <select
    :aria-label="t('Язык')"
    :value="locale"
    @change="setLocale(($event.target as HTMLSelectElement).value)"
    class="rounded-lg border border-border-primary bg-card px-2 py-1 text-sm"
  >
    <option value="ru">Русский</option>
    <option value="en">English</option>
  </select>
</template>
