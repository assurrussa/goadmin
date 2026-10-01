<template>
  <div class="rounded-md border border-border-primary overflow-hidden bg-card">
    <div class="relative w-full overflow-auto">
      <Table>
        <TableHeader class="bg-surface">
          <TableRow class="hover:bg-transparent border-border-primary">
            <TableHead
              v-if="config.behaviour?.selectable"
              class="w-12 px-6 sm:w-16 sm:px-8 sticky left-0 bg-surface z-20"
            >
              <Checkbox
                :checked="allSelected"
                aria-label="Выбрать все строки"
                class="absolute left-4 top-1/2 -mt-2 sm:left-6"
                @update:checked="$emit('toggle-select-all')"
              />
            </TableHead>
            <TableHead
              v-for="column in config.columns"
              :key="column.key"
              class="group px-6 py-3 text-left text-xs font-medium text-text-tertiary uppercase tracking-wider cursor-pointer whitespace-nowrap h-auto"
              :class="{
                'cursor-pointer hover:bg-surface-variant transition-colors': column.sortable,
              }"
              @click="column.sortable && $emit('sort', column.key)"
            >
              <div class="flex items-center space-x-1">
                <span>{{ column.label }}</span>
                <span
                  v-if="column.sortable"
                  class="flex-none rounded text-text-tertiary group-hover:visible"
                  :class="sortBy === column.key ? 'text-primary' : ''"
                >
                  <ArrowUp v-if="sortBy === column.key && sortOrder === 'asc'" class="h-4 w-4" />
                  <ArrowDown
                    v-else-if="sortBy === column.key && sortOrder === 'desc'"
                    class="h-4 w-4"
                  />
                  <ArrowUpDown v-else class="h-4 w-4 opacity-60" />
                </span>
              </div>
            </TableHead>
            <!-- Sticky Actions Column Header -->
            <TableHead
              v-if="hasAnyActions"
              class="relative px-6 py-3 sticky right-0 bg-surface z-20 whitespace-nowrap h-auto border-l border-border-primary shadow-[-12px_0_18px_-16px_rgba(15,23,42,0.35)]"
            >
              <span class="sr-only">Действия</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody class="bg-card">
          <TableRow
            v-for="item in items"
            :key="getItemId(item, config)"
            class="hover:bg-surface transition-colors duration-200 border-border-primary"
          >
            <TableCell
              v-if="config.behaviour?.selectable"
              class="relative w-12 px-6 sm:w-16 sm:px-8 sticky left-0 bg-card z-10"
            >
              <Checkbox
                :checked="selectedItems.includes(getItemId(item, config))"
                :aria-label="`Выбрать строку ${getItemId(item, config)}`"
                class="absolute left-4 top-1/2 -mt-2 sm:left-6"
                @update:checked="$emit('toggle-select-item', getItemId(item, config))"
              />
            </TableCell>
            <TableCell
              v-for="column in config.columns"
              :key="column.key"
              class="px-6 py-4 whitespace-nowrap text-sm text-text-primary"
            >
              <!-- Badge for select type -->
              <AppBadge
                v-if="column.type === 'select' && column.badges"
                :variant="getBadgeVariant(getColumnValue(item, column.key), column.badges)"
                class="inline-flex"
              >
                {{ getBadgeLabel(getColumnValue(item, column.key), column.badges) }}
              </AppBadge>
              <!-- Boolean values -->
              <span
                v-else-if="column.type === 'boolean'"
                :class="getColumnValue(item, column.key) ? 'text-success' : 'text-error'"
                class="inline-flex items-center"
              >
                <CheckCircle2 v-if="getColumnValue(item, column.key)" class="h-4 w-4 mr-1" />
                <XCircle v-else class="h-4 w-4 mr-1" />
                {{ getColumnValue(item, column.key) ? 'Да' : 'Нет' }}
              </span>
              <!-- Rich Text values -->
              <div
                v-else-if="column.type === 'richtext'"
                class="richtext-preview max-w-xs overflow-hidden"
              >
                <div
                  v-if="isRichTextJSON(getColumnValue(item, column.key))"
                  class="prose prose-sm max-w-none text-ellipsis"
                  v-html="renderRichTextPreview(getColumnValue(item, column.key))"
                ></div>
                <span v-else class="text-text-tertiary italic">
                  {{ formatValue(getColumnValue(item, column.key), column) }}
                </span>
              </div>
              <!-- Standard value -->
              <span v-else class="font-medium">
                {{ formatValue(getColumnValue(item, column.key), column) }}
              </span>
            </TableCell>
            <!-- Sticky Actions Column Cell -->
            <TableCell
              v-if="item.actions && item.actions.length > 0"
              class="px-6 py-4 whitespace-nowrap text-right text-sm font-medium sticky right-0 bg-card z-10 border-l border-border-primary shadow-[-12px_0_18px_-16px_rgba(15,23,42,0.35)]"
            >
              <div class="flex items-center justify-end gap-1">
                <template v-for="action in item.actions" :key="action.key">
                  <AppButton
                    v-if="['edit', 'show', 'view', 'delete'].includes(action.key)"
                    variant="ghost"
                    size="icon-sm"
                    class="h-8 w-8 p-0"
                    :class="getActionClass(action)"
                    @click="$emit('action', action.key, item.item)"
                    :title="action.label"
                    :aria-label="action.label"
                  >
                    <Pencil v-if="action.key === 'edit'" class="h-4 w-4" />
                    <Eye v-else-if="['show', 'view'].includes(action.key)" class="h-4 w-4" />
                    <Trash2 v-else-if="action.key === 'delete'" class="h-4 w-4 text-destructive" />
                    <component :is="action.icon" v-else-if="action.icon" class="h-4 w-4" />
                    <span v-else class="sr-only">{{ action.label }}</span>
                  </AppButton>
                </template>

                <DropdownMenu
                  v-if="
                    item.actions.filter((a) => !['edit', 'show', 'view', 'delete'].includes(a.key))
                      .length > 0
                  "
                >
                  <DropdownMenuTrigger as-child>
                    <AppButton variant="ghost" size="icon-sm" class="h-8 w-8 p-0">
                      <span class="sr-only">Еще</span>
                      <MoreHorizontal class="h-4 w-4" />
                    </AppButton>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuLabel>Действия</DropdownMenuLabel>
                    <template v-for="action in item.actions" :key="action.key">
                      <DropdownMenuItem
                        v-if="!['edit', 'show', 'view', 'delete'].includes(action.key)"
                        @click="$emit('action', action.key, item.item)"
                        :class="
                          action.variant === 'danger'
                            ? 'text-destructive focus:text-destructive'
                            : ''
                        "
                      >
                        {{ action.label }}
                      </DropdownMenuItem>
                    </template>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { Config, Column, DataItem, DataItemValue } from '~/composables/useDataGrid.ts'
import { computed } from 'vue'
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  CheckCircle2,
  Eye,
  MoreHorizontal,
  Pencil,
  Trash2,
  XCircle,
} from 'lucide-vue-next'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

// Props interface
interface Props {
  config: Config
  items: DataItem[]
  sortBy: string
  sortOrder: 'asc' | 'desc'
  selectedItems: (string | number)[]
  allSelected: boolean
}

// Define props with defaults
const props = withDefaults(defineProps<Props>(), {
  config: () => ({}),
  items: () => [],
  sortBy: '',
  sortOrder: 'desc',
  selectedItems: () => [],
  allSelected: false,
})

// Define emits
defineEmits<{
  sort: [columnKey: string]
  'toggle-select-all': []
  'toggle-select-item': [id: string | number]
  action: [actionKey: string, item: DataItemValue]
  'inline-create': []
}>()

// Computed properties
const hasAnyActions = computed((): boolean => {
  return Object.values(props.items).some((item) => item.actions && item.actions.length > 0)
})

const getActionClass = (action: { key: string; variant?: string }) => {
  if (action.variant === 'danger' || action.key === 'delete') {
    return 'text-destructive hover:text-destructive'
  }
  if (action.key === 'edit') {
    return 'text-warning hover:text-warning'
  }
  if (action.key === 'show' || action.key === 'view') {
    return 'text-info hover:text-info'
  }
  return 'text-primary hover:text-primary'
}

// Helper function to safely get item ID
const getItemId = (item: DataItem, config: Config): string | number => {
  const id = item.item[config.ui?.idKey || 'id']
  return typeof id === 'string' || typeof id === 'number' ? id : String(id)
}

// Helper function to safely get column value
const getColumnValue = (item: DataItem, columnKey: string): string | number | boolean => {
  const hasComputedValue =
    item.values && Object.prototype.hasOwnProperty.call(item.values, columnKey)
  const source = hasComputedValue ? item.values! : item.item
  const value = source[columnKey]

  if (typeof value === 'string' || typeof value === 'number') {
    return value
  }

  if (typeof value === 'boolean') {
    return value
  }

  if (value === null || value === undefined) {
    return ''
  }

  return String(value)
}

const formatValue = (value: unknown, column: Column): string => {
  if (!value) return ''

  if (column.type === 'date' && column.format) {
    return new Date(value as string).toLocaleDateString('ru-RU', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  return String(value)
}

const getBadgeVariant = (
  value: unknown,
  badges: Record<string | number, { label: string; variant: string }>,
): 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'info' | 'outline' => {
  const safeValue = typeof value === 'string' || typeof value === 'number' ? value : String(value)
  const badge = badges[safeValue]
  if (!badge) return 'secondary'

  const variants: Record<
    string,
    'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'info' | 'outline'
  > = {
    success: 'success',
    danger: 'danger',
    warning: 'warning',
    info: 'info',
    primary: 'primary',
    secondary: 'secondary',
    outline: 'outline',
  }

  return variants[badge.variant] || 'secondary'
}

const getBadgeLabel = (
  value: unknown,
  badges: Record<string | number, { label: string; variant: string }>,
): string => {
  const safeValue = typeof value === 'string' || typeof value === 'number' ? value : String(value)
  return badges[safeValue]?.label || String(value)
}

// Rich Text helper functions
interface ProsemirrorNode {
  type: string
  content?: ProsemirrorNode[]
  attrs?: Record<string, unknown>
  text?: string
  marks?: { type: string; attrs?: Record<string, unknown> }[]
}

const isRichTextJSON = (value: unknown): boolean => {
  if (typeof value !== 'string') return false

  try {
    const parsed = JSON.parse(value) as { type?: string }
    return !!(parsed && typeof parsed === 'object' && parsed.type === 'doc')
  } catch {
    return false
  }
}

const renderRichTextPreview = (value: unknown): string => {
  if (typeof value !== 'string') return ''

  try {
    const json = JSON.parse(value)
    // Простой рендеринг JSON в HTML для превью
    return jsonToHtml(json)
  } catch {
    return String(value)
  }
}

const jsonToHtml = (json: unknown): string => {
  if (!json || typeof json !== 'object') return ''
  const node = json as ProsemirrorNode

  if (node.type === 'doc' && node.content) {
    return node.content.map((child) => renderNode(child)).join('')
  }

  return renderNode(node)
}

const renderNode = (node: ProsemirrorNode): string => {
  if (!node || typeof node !== 'object') return ''

  switch (node.type) {
    case 'paragraph':
      return `<p>${node.content ? node.content.map(renderNode).join('') : ''}</p>`
    case 'heading':
      const level = node.attrs?.level || 1
      return `<h${level}>${node.content ? node.content.map(renderNode).join('') : ''}</h${level}>`
    case 'text':
      let html = node.text || ''
      if (node.marks) {
        for (const mark of node.marks) {
          switch (mark.type) {
            case 'bold':
              html = `<strong>${html}</strong>`
              break
            case 'italic':
              html = `<em>${html}</em>`
              break
            case 'underline':
              html = `<u>${html}</u>`
              break
            case 'strike':
              html = `<s>${html}</s>`
              break
            case 'code':
              html = `<code>${html}</code>`
              break
            case 'link':
              const href = (mark.attrs?.href as string) || ''
              html = `<a href="${href}">${html}</a>`
              break
          }
        }
      }
      return html
    case 'bulletList':
      return `<ul>${node.content ? node.content.map(renderNode).join('') : ''}</ul>`
    case 'orderedList':
      return `<ol>${node.content ? node.content.map(renderNode).join('') : ''}</ol>`
    case 'listItem':
      return `<li>${node.content ? node.content.map(renderNode).join('') : ''}</li>`
    case 'blockquote':
      return `<blockquote>${node.content ? node.content.map(renderNode).join('') : ''}</blockquote>`
    case 'codeBlock':
      return `<pre><code>${node.content ? node.content.map(renderNode).join('') : ''}</code></pre>`
    case 'hardBreak':
      return '<br>'
    default:
      return node.content ? node.content.map(renderNode).join('') : ''
  }
}
</script>
