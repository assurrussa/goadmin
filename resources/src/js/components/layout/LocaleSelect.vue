<script setup lang="ts">
import { appearance } from '@/appearance/state'
import { Languages } from 'lucide-vue-next'
import { useI18n } from '@/i18n'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@/components/ui/dropdown-menu'
const { t, locale, setLocale } = useI18n()
function selectLocale(value: unknown) {
  if (value === 'ru' || value === 'en') setLocale(value)
}
</script>
<template>
  <DropdownMenu>
    <DropdownMenuTrigger
      :disabled="appearance.readOnly || appearance.resolving"
      class="appearance-select inline-flex shrink-0 items-center justify-center gap-2 rounded-[var(--goadmin-control-radius,4px)] border border-border-primary bg-card max-sm:min-w-11 sm:px-3 text-sm text-text-primary hover:bg-surface-variant focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      :aria-label="t('Язык')"
      :title="locale === 'ru' ? 'Русский' : 'English'"
    >
      <Languages class="hidden h-4 w-4 sm:block" aria-hidden="true" />
      <span>{{ locale === 'ru' ? 'RU' : 'EN' }}</span>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" class="min-w-40">
      <DropdownMenuRadioGroup :model-value="locale" @update:model-value="selectLocale">
        <DropdownMenuRadioItem value="ru">Русский</DropdownMenuRadioItem>
        <DropdownMenuRadioItem value="en">English</DropdownMenuRadioItem>
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
