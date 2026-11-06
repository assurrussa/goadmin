<script setup lang="ts">
import { computed } from 'vue'
import { appearance } from '@/appearance/state'
import { t } from '@/i18n'
import { useThemeStore } from '@/stores/theme'
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import AppButton from '@/components/ui/AppButton.vue'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu'
const theme = useThemeStore()
const title = computed(() =>
  theme.preference === 'system'
    ? t(theme.isDark ? 'Системная (тёмная)' : 'Системная (светлая)')
    : t(theme.isDark ? 'Тёмная' : 'Светлая'),
)
function selectTheme(value: unknown) {
  if (value === 'system' || value === 'light' || value === 'dark') theme.setTheme(value)
}
</script>
<template>
  <DropdownMenu>
    <DropdownMenuTrigger as-child :disabled="appearance.readOnly || appearance.resolving">
      <AppButton
        variant="ghost"
        size="icon-sm"
        :disabled="appearance.readOnly || appearance.resolving"
        :aria-label="t('Выбрать тему') + ': ' + title"
        :title="title"
      >
        <Monitor v-if="theme.preference === 'system'" class="h-5 w-5" aria-hidden="true" />
        <Moon v-else-if="theme.isDark" class="h-5 w-5" aria-hidden="true" />
        <Sun v-else class="h-5 w-5" aria-hidden="true" />
      </AppButton>
    </DropdownMenuTrigger>
    <DropdownMenuContent :aria-label="t('Тема')" align="end" class="min-w-44">
      <DropdownMenuRadioGroup :model-value="theme.preference" @update:model-value="selectTheme">
        <DropdownMenuRadioItem
          :disabled="appearance.readOnly || appearance.resolving"
          value="system"
          >{{ t('Системная') }}</DropdownMenuRadioItem
        >
        <DropdownMenuRadioItem
          :disabled="appearance.readOnly || appearance.resolving"
          value="light"
          >{{ t('Светлая') }}</DropdownMenuRadioItem
        >
        <DropdownMenuRadioItem
          :disabled="appearance.readOnly || appearance.resolving"
          value="dark"
          >{{ t('Тёмная') }}</DropdownMenuRadioItem
        >
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
