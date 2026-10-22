<script setup lang="ts">
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import { Menu } from 'lucide-vue-next'
import AdminHeaderUtilities from './AdminHeaderUtilities.vue'
import AdminAccountMenu from './AdminAccountMenu.vue'
import { provideAdminHeaderState } from './useAdminHeader'
defineProps<{ sidebarOpen?: boolean }>()
defineEmits<{ 'toggle-sidebar': [] }>()
provideAdminHeaderState()
</script>
<template>
  <header
    data-ui="admin-header"
    class="box-border h-[var(--goadmin-header-height,68px)] shrink-0 bg-card border-b border-border-primary shadow-[var(--goadmin-header-shadow,var(--goadmin-default-header-shadow))]"
  >
    <div
      class="flex items-center justify-between px-4 py-1 sm:px-6 lg:px-8 max-w-[var(--goadmin-content-max-width,80rem)] mx-auto w-full h-full min-h-0"
    >
      <div class="flex items-center">
        <slot name="navigationToggle">
          <AppButton
            variant="ghost"
            size="icon-sm"
            class="lg:hidden"
            :aria-label="t('Открыть меню')"
            :aria-expanded="sidebarOpen"
            aria-controls="admin-mobile-navigation"
            @click="$emit('toggle-sidebar')"
            ><Menu class="h-5 w-5"
          /></AppButton>
        </slot>
      </div>
      <div class="flex items-center gap-2 sm:gap-3">
        <AdminHeaderUtilities /><AdminAccountMenu />
      </div>
    </div>
  </header>
</template>
