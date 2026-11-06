<script setup lang="ts">
import { computed } from 'vue'
import { appearance } from '@/appearance/state'
import { t } from '@/i18n'
import { useThemeStore } from '@/stores/theme'
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import AppButton from '@/components/ui/AppButton.vue'
const theme = useThemeStore()
const title = computed(() =>
  theme.preference === 'system'
    ? t(theme.isDark ? 'Системная (тёмная)' : 'Системная (светлая)')
    : t(theme.preference === 'dark' ? 'Тёмная' : 'Светлая'),
)
function cycleTheme() {
  const next = { system: 'light', light: 'dark', dark: 'system' } as const
  theme.setTheme(next[theme.preference])
}
</script>
<template>
  <AppButton
    variant="ghost"
    size="icon-sm"
    :disabled="appearance.readOnly || appearance.resolving"
    :aria-label="t('Выбрать тему') + ': ' + title"
    :title="title"
    @click="cycleTheme"
  >
    <Monitor v-if="theme.preference === 'system'" class="h-5 w-5" aria-hidden="true" />
    <Sun v-else-if="theme.preference === 'light'" class="h-5 w-5" aria-hidden="true" />
    <Moon v-else class="h-5 w-5" aria-hidden="true" />
  </AppButton>
</template>
