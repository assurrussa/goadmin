<template>
  <div class="flex flex-col h-full">
    <!-- Logo -->
    <div
      class="flex h-[var(--goadmin-header-height,68px)] shrink-0 items-center gap-2 px-3 border-b border-border-primary"
      :class="collapsed ? 'justify-center' : 'justify-between'"
    >
      <div :class="collapsed ? 'hidden' : 'flex min-w-0 items-center space-x-3 whitespace-nowrap'">
        <AdminBrand />
      </div>

      <AppButton
        variant="ghost"
        size="icon-sm"
        v-if="!mobile"
        class="hidden lg:inline-flex"
        :aria-label="collapsed ? t('Развернуть') : t('Свернуть')"
        :title="collapsed ? t('Развернуть') : t('Свернуть')"
        :aria-expanded="!collapsed"
        @click="$emit('toggle-collapse')"
        ><PanelLeft class="h-5 w-5"
      /></AppButton>
      <!-- Close button for mobile -->
      <AppButton
        variant="ghost"
        size="icon-sm"
        v-if="mobile"
        class="lg:hidden"
        :aria-label="t('Закрыть меню')"
        @click="$emit('close')"
      >
        <X class="h-5 w-5" />
      </AppButton>
    </div>

    <!-- Navigation -->
    <nav
      data-ui="navigation"
      data-part="vertical"
      class="flex-1 min-h-0 px-0 py-3 space-y-1 overflow-y-auto overflow-x-hidden"
    >
      <template v-for="(section, index) in sections" :key="section.key">
        <div class="space-y-1 px-2">
          <div v-if="section.title" :class="collapsed ? 'sr-only' : 'px-2 py-2'">
            <h3 class="text-[11px] font-semibold text-text-tertiary uppercase tracking-widest">
              {{ t(section.title) }}
            </h3>
          </div>
          <SidebarMenuItems
            :items="section.items"
            :depth="0"
            :collapsed="collapsed"
            :is-active="isActive"
            :resolve-icon="resolveIcon"
          />
        </div>

        <div
          v-if="index < sections.length - 1"
          class="border-t border-border-primary"
          :class="collapsed ? 'my-2' : 'my-4'"
        ></div>
      </template>
    </nav>

    <!-- Footer -->
    <footer :class="collapsed ? 'sr-only' : 'shrink-0 border-t border-border-primary p-4'">
      <p class="text-xs font-medium text-text-secondary">{{ t('Об админке') }}</p>
      <p class="mt-1 text-xs text-text-tertiary break-words">
        {{ t('GoAdmin · версия') }} {{ adminVersion }}
      </p>
    </footer>
  </div>
</template>

<script setup lang="ts">
import AdminBrand from './AdminBrand.vue'
import { t } from '@/i18n'
import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'
import AppButton from '@/components/ui/AppButton.vue'
import { X, PanelLeft } from 'lucide-vue-next'
import SidebarMenuItems from './SidebarMenuItems.vue'
import { useAdminNavigation } from './useAdminNavigation'
defineEmits<{ close: []; 'toggle-collapse': [] }>()
defineProps<{ collapsed?: boolean; mobile?: boolean }>()
const page = usePage()
const { sections, resolveIcon, isActive } = useAdminNavigation()
const adminVersion = computed(() => {
  const info = page.props.adminInfo as { version?: string } | undefined
  if (info?.version === 'dev') return t('разработка')
  return info?.version || t('неизвестна')
})
</script>
