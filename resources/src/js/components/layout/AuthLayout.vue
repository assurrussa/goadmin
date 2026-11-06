<script setup lang="ts">
import AdminBrand from './AdminBrand.vue'
import { usePresentation } from '@/presentation/context'
import LocaleSelect from './LocaleSelect.vue'
import ThemeSelect from './ThemeSelect.vue'
const { authShell } = usePresentation()
</script>
<template>
  <component :is="authShell">
    <template #utilities>
      <LocaleSelect />
      <ThemeSelect />
    </template>
    <template #brand><AdminBrand /></template>
    <template #title><slot name="title" /></template>
    <template v-if="$slots.subtitle" #subtitle><slot name="subtitle" /></template>
    <template v-if="$slots.footer" #footer><slot name="footer" /></template>
    <slot />
  </component>
</template>
