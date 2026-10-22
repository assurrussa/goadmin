<script setup lang="ts">
import { t } from '@/i18n'
import { usePresentation } from '@/presentation/context'
import LocaleSelect from './LocaleSelect.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Moon, Sun } from 'lucide-vue-next'
import { useThemeStore } from '@/stores/theme'
const { authShell } = usePresentation()
const themeStore = useThemeStore()
</script>
<template>
  <component :is="authShell">
    <template #utilities>
      <LocaleSelect />
      <AppButton
        variant="ghost"
        size="icon-sm"
        :aria-label="
          themeStore.isDark ? t('Переключить на светлую тему') : t('Переключить на темную тему')
        "
        :title="
          themeStore.isDark ? t('Переключить на светлую тему') : t('Переключить на темную тему')
        "
        @click="themeStore.toggle()"
      >
        <Sun v-if="themeStore.isDark" class="h-5 w-5" /><Moon v-else class="h-5 w-5" />
      </AppButton>
    </template>
    <template #brand
      ><span class="text-sm font-semibold text-text-secondary">{{ t('Админка') }}</span></template
    >
    <template #title><slot name="title" /></template>
    <template v-if="$slots.subtitle" #subtitle><slot name="subtitle" /></template>
    <template v-if="$slots.footer" #footer><slot name="footer" /></template>
    <slot />
  </component>
</template>
