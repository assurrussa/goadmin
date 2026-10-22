<script setup lang="ts">
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ChevronDown } from 'lucide-vue-next'
import AdminTopMenuItems from './AdminTopMenuItems.vue'
import { useAdminNavigation } from './useAdminNavigation'
const { sections, isBranchActive } = useAdminNavigation()

function revealFocusedTrigger(event: FocusEvent) {
  const navigation = event.currentTarget
  const trigger = event.target
  if (!(navigation instanceof HTMLElement) || !(trigger instanceof HTMLElement)) return

  // Run after the browser's own focus scrolling, and adjust this strip only.
  queueMicrotask(() => {
    if (
      !navigation.isConnected ||
      document.activeElement !== trigger ||
      !trigger.matches(':focus-visible')
    )
      return
    const bounds = navigation.getBoundingClientRect()
    const focused = trigger.getBoundingClientRect()
    // Keep the focus outline visible as well as the complete trigger label.
    const left = bounds.left + navigation.clientLeft + 4
    const right = bounds.left + navigation.clientLeft + navigation.clientWidth - 4
    const delta = focused.left < left ? focused.left - left : Math.max(0, focused.right - right)
    if (delta) navigation.scrollLeft += delta
  })
}
</script>
<template>
  <nav
    data-ui="navigation"
    data-part="horizontal"
    :aria-label="t('Разделы')"
    @focusin="revealFocusedTrigger"
    class="flex min-w-0 items-center gap-2 overflow-x-auto p-2"
  >
    <DropdownMenu v-for="section in sections" :key="section.key">
      <DropdownMenuTrigger as-child>
        <AppButton
          variant="ghost"
          class="shrink-0 whitespace-nowrap"
          :data-active="section.items.some(isBranchActive) ? '' : undefined"
          :class="section.items.some(isBranchActive) ? 'bg-primary/10 text-primary' : undefined"
          >{{ t(section.title || section.items[0]?.name || 'Разделы') }}<ChevronDown class="size-4"
        /></AppButton>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" class="max-h-[70vh] overflow-y-auto">
        <AdminTopMenuItems :items="section.items" />
      </DropdownMenuContent>
    </DropdownMenu>
  </nav>
</template>
