<script setup lang="ts">
import { ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { router } from '@inertiajs/vue3'
import { t } from '@/i18n'
import AdminSidebar from './AdminSidebar.vue'
const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()
const panel = ref<HTMLElement | null>(null)
let previousFocus: HTMLElement | null = null
let previousOverflow = ''
let locked = false
let viewport: MediaQueryList | undefined
const restore = () => {
  if (!locked) return
  locked = false
  document.body.style.overflow = previousOverflow
  if (previousFocus?.isConnected) previousFocus.focus()
}
watch(
  () => props.open,
  async (open) => {
    if (!open) {
      restore()
      return
    }
    previousFocus = document.activeElement as HTMLElement | null
    previousOverflow = document.body.style.overflow
    locked = true
    document.body.style.overflow = 'hidden'
    await nextTick()
    if (props.open)
      panel.value?.querySelector<HTMLElement>('button:not([disabled]), a[href]')?.focus()
  },
)
const keydown = (event: KeyboardEvent) => {
  if (!props.open) return
  if (event.key === 'Escape') {
    event.preventDefault()
    emit('close')
    return
  }
  if (event.key !== 'Tab') return
  const nodes = Array.from(
    panel.value?.querySelectorAll<HTMLElement>(
      'a[href],button:not([disabled]),select:not([disabled]),[tabindex="0"]',
    ) ?? [],
  ).filter((el) => el.getClientRects().length > 0 && !el.closest('[inert]'))
  const first = nodes[0],
    last = nodes.at(-1)
  if (!first) {
    event.preventDefault()
    return
  }
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last?.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
const closeForViewport = () => emit('close')
const stopNavigation = router.on('navigate', closeForViewport)
onMounted(() => {
  document.addEventListener('keydown', keydown)
  viewport = window.matchMedia('(max-width: 1023px)')
  viewport.addEventListener('change', closeForViewport)
})
onUnmounted(() => {
  stopNavigation()
  document.removeEventListener('keydown', keydown)
  viewport?.removeEventListener('change', closeForViewport)
  restore()
})
</script>
<template>
  <Teleport to="body">
    <div
      v-if="open"
      data-ui="mobile-navigation-overlay"
      class="fixed inset-0 z-40 lg:hidden backdrop-blur-sm"
      style="background-color: var(--color-overlay)"
      @click="emit('close')"
    />
    <div
      ref="panel"
      id="admin-mobile-navigation"
      data-ui="mobile-navigation"
      :inert="open ? undefined : true"
      :aria-hidden="!open"
      :role="open ? 'dialog' : undefined"
      :aria-modal="open ? true : undefined"
      :aria-label="t('Разделы')"
      class="fixed inset-y-0 left-0 z-50 lg:hidden w-[var(--goadmin-sidebar-width,16rem)] bg-card border-r border-border-primary overflow-x-hidden transition-transform duration-200 motion-reduce:transition-none"
      :class="open ? 'translate-x-0' : '-translate-x-full'"
    >
      <AdminSidebar mobile @close="emit('close')" />
    </div>
  </Teleport>
</template>
