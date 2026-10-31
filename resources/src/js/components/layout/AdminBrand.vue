<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { appearance } from '@/appearance/state'
import { t } from '@/i18n'
const logo = computed(() =>
  appearance.dark
    ? appearance.brand.logoDarkUrl || appearance.brand.logoLightUrl
    : appearance.brand.logoLightUrl || appearance.brand.logoDarkUrl,
)
const failed = ref(false)
watch(logo, () => {
  failed.value = false
})
</script>
<template>
  <span data-ui="admin-brand" class="flex min-w-0 items-center gap-3">
    <img
      v-if="logo && !failed"
      :src="logo"
      alt=""
      class="h-9 w-9 shrink-0 rounded-lg object-contain"
      @error="failed = true"
    />
    <span
      v-else
      class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 font-semibold text-primary"
      aria-hidden="true"
      >{{ (appearance.brand.name || 'A').slice(0, 1) }}</span
    >
    <span class="truncate text-sm font-semibold text-text-primary">{{
      appearance.brand.name || t('Админка')
    }}</span>
  </span>
</template>
