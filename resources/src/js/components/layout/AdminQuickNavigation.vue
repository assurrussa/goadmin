<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, useId, watch } from 'vue'
import { router } from '@inertiajs/vue3'
import { Search } from 'lucide-vue-next'
import { locale, t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useAdminHeader } from './useAdminHeader'
import { isLocalAdminMenuHref, useAdminNavigation } from './useAdminNavigation'

type MenuItem = { name: string; href?: string; children?: MenuItem[] }
type Destination = { label: string; context: string; href: string }

const { sections } = useAdminNavigation()
const { accountMenuItems } = useAdminHeader()
const open = ref(false)
const query = ref('')
const searchInput = ref<HTMLInputElement>()
const resultList = ref<HTMLUListElement>()
const activeIndex = ref(0)
const resultsId = useId()
let shortcutOpener: HTMLElement | undefined
const shortcutLabel = ref('Ctrl+K')

const destinations = computed(() => {
  const entries: Destination[] = []
  function collect(items: MenuItem[], parents: string[]) {
    for (const item of items) {
      const label = t(item.name)
      if (
        isLocalAdminMenuHref(item.href) &&
        item.href.split(/[?#]/)[0]?.replace(/\/+$/, '') !== '/auth/logout'
      ) {
        entries.push({ label, context: [...parents, item.href].join(' · '), href: item.href })
      }
      if (item.children?.length) collect(item.children, [...parents, label])
    }
  }

  for (const section of sections.value) {
    collect(section.items, section.title ? [t(section.title)] : [])
  }
  collect(
    [
      { name: 'Профиль', href: '/auth/profile' },
      { name: 'Настройки', href: '/auth/profile/settings' },
      ...accountMenuItems.value,
    ],
    [],
  )
  return entries
})

function normalize(value: string): string {
  return value.normalize('NFKC').toLocaleLowerCase(locale.value)
}

const results = computed(() => {
  const terms = normalize(query.value).trim().split(/\s+/).filter(Boolean)
  return destinations.value.filter(({ label, context }) => {
    const searchable = normalize(`${label} ${context}`)
    return terms.every((term) => searchable.includes(term))
  })
})
const optionId = (index: number) => `${resultsId}-${index}`
watch(
  results,
  () => {
    activeIndex.value = 0
    if (resultList.value) resultList.value.scrollTop = 0
  },
  { flush: 'sync' },
)
watch(open, (value) => {
  if (!value) {
    query.value = ''
    activeIndex.value = 0
  }
})

function visit(destination: Destination) {
  router.get(destination.href, {}, { onStart: () => (open.value = false) })
}

function onSearchKeydown(event: KeyboardEvent) {
  if (event.isComposing || !results.value.length) return
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const count = results.value.length
    const change = event.key === 'ArrowDown' ? 1 : -1
    activeIndex.value = (activeIndex.value + change + count) % count
    document.getElementById(optionId(activeIndex.value))?.scrollIntoView?.({ block: 'nearest' })
  } else if (event.key === 'Enter' && !event.repeat) {
    event.preventDefault()
    const destination = results.value[activeIndex.value]
    if (destination) visit(destination)
  }
}

const editorSelector =
  'input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="textbox"], [role="combobox"]'

function openWithShortcut(event: KeyboardEvent) {
  if (
    open.value ||
    event.defaultPrevented ||
    event.repeat ||
    event.isComposing ||
    event.altKey ||
    event.shiftKey ||
    !(event.ctrlKey || event.metaKey) ||
    (event.code !== 'KeyK' && !['k', 'л'].includes(event.key.toLowerCase()))
  )
    return

  const active = document.activeElement
  if (
    active?.closest(editorSelector) ||
    event.composedPath().some((node) => node instanceof Element && node.matches(editorSelector))
  )
    return
  const dialogs = document.querySelectorAll('dialog[open], [role="dialog"], [role="alertdialog"]')
  if (
    Array.from(dialogs).some(
      (dialog) => !dialog.closest('[hidden], [inert], [aria-hidden="true"], [data-state="closed"]'),
    )
  )
    return

  event.preventDefault()
  shortcutOpener = active instanceof HTMLElement && active !== document.body ? active : undefined
  open.value = true
}

function focusSearch(event: Event) {
  event.preventDefault()
  searchInput.value?.focus()
}

function restoreShortcutFocus(event: Event) {
  const opener = shortcutOpener
  shortcutOpener = undefined
  if (opener?.isConnected) {
    opener.focus()
    if (document.activeElement === opener) event.preventDefault()
  }
}

onMounted(() => {
  if (/Macintosh|iPhone|iPad|iPod/.test(navigator.userAgent)) shortcutLabel.value = '⌘K'
  document.addEventListener('keydown', openWithShortcut)
})
onUnmounted(() => document.removeEventListener('keydown', openWithShortcut))
</script>

<template>
  <Dialog v-model:open="open">
    <DialogTrigger as-child>
      <AppButton
        data-quick-navigation-trigger
        variant="ghost"
        size="icon-sm"
        class="shrink-0 lg:w-auto lg:px-2"
        :aria-label="t('Перейти к разделу')"
        aria-keyshortcuts="Control+K Meta+K"
      >
        <Search class="size-4" aria-hidden="true" />
        <span class="hidden lg:inline">{{ t('Перейти к разделу') }}</span>
        <kbd
          aria-hidden="true"
          class="hidden rounded border border-border-primary px-1 text-xs text-text-secondary lg:inline"
          >{{ shortcutLabel }}</kbd
        >
      </AppButton>
    </DialogTrigger>

    <DialogContent
      data-quick-navigation
      aria-modal="true"
      class="max-h-[calc(100dvh-2rem)] overflow-y-auto"
      @open-auto-focus="focusSearch"
      @close-auto-focus="restoreShortcutFocus"
    >
      <DialogTitle class="pr-6">{{ t('Быстрый переход') }}</DialogTitle>
      <DialogDescription class="sr-only">{{ t('Название раздела или путь') }}</DialogDescription>
      <input
        ref="searchInput"
        v-model="query"
        type="text"
        role="combobox"
        aria-autocomplete="list"
        :aria-label="t('Поиск разделов')"
        :aria-controls="resultsId"
        :aria-expanded="results.length > 0"
        :aria-activedescendant="results.length ? optionId(activeIndex) : undefined"
        :placeholder="t('Название раздела или путь')"
        autocomplete="off"
        class="h-10 w-full rounded-md border border-border-primary bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        @keydown="onSearchKeydown"
      />
      <ul
        v-show="results.length"
        ref="resultList"
        :id="resultsId"
        :aria-label="t('Результаты поиска')"
        role="listbox"
        class="max-h-[50dvh] space-y-1 overflow-y-auto"
      >
        <li
          v-for="(entry, index) in results"
          :id="optionId(index)"
          :key="index"
          role="option"
          :aria-selected="index === activeIndex"
          class="cursor-pointer rounded-md px-3 py-2 text-sm hover:bg-surface-variant"
          :class="index === activeIndex && 'goadmin-selection-fill bg-primary/10'"
          @click="visit(entry)"
        >
          <span class="block truncate font-medium">{{ entry.label }}</span>
          <span class="block truncate text-xs text-text-secondary">{{ entry.context }}</span>
        </li>
      </ul>
      <p v-if="!results.length" role="status" class="text-sm text-text-secondary">
        {{ t('Совпадений нет') }}
      </p>
    </DialogContent>
  </Dialog>
</template>
