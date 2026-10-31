<script setup lang="ts">
import { appearance } from '@/appearance/state'
import { t } from '@/i18n'
import { Link } from '@inertiajs/vue3'
import RolePreviewBanner from './RolePreviewBanner.vue'
import LocaleSelect from './LocaleSelect.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Badge } from '@/components/ui/badge'
import { Bell, Moon, Sun } from 'lucide-vue-next'
import { useThemeStore } from '@/stores/theme'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { useAdminHeader } from './useAdminHeader'
const { rolePreviewOpen, unreadNotifications } = useAdminHeader()
const { notifications } = useAdminCapabilities()
const themeStore = useThemeStore()
const handleThemeToggle = () => themeStore.toggle()
</script>
<template>
  <RolePreviewBanner v-model:open="rolePreviewOpen" />
  <LocaleSelect />
  <AppButton
    variant="ghost"
    size="icon-sm"
    :disabled="appearance.readOnly || appearance.resolving"
    :aria-label="
      themeStore.isDark ? t('Переключить на светлую тему') : t('Переключить на темную тему')
    "
    :title="themeStore.isDark ? t('Переключить на светлую тему') : t('Переключить на темную тему')"
    @click="handleThemeToggle"
  >
    <Sun v-if="themeStore.isDark" class="h-5 w-5" />
    <Moon v-else class="h-5 w-5" />
  </AppButton>

  <Link v-if="notifications" href="/notifications" class="relative">
    <AppButton variant="ghost" size="icon-sm" :aria-label="t('Открыть уведомления')">
      <Bell class="h-5 w-5" />
    </AppButton>
    <Badge
      v-if="unreadNotifications > 0"
      variant="destructive"
      class="absolute -top-1 -right-1 min-w-[16px] h-4 px-1 rounded-full text-[10px] flex items-center justify-center p-0"
    >
      {{ unreadNotifications }}
    </Badge>
  </Link>
</template>
