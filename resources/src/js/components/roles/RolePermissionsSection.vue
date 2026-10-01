<template>
  <section v-if="groupedKeys.length" :class="wrapperClass">
    <div v-if="showHeader" :class="headerClass">
      <div>
        <p v-if="headerEyebrow" class="text-xs uppercase tracking-wide text-text-tertiary mb-1">
          {{ headerEyebrow }}
        </p>
        <h3 class="text-base font-semibold text-text-primary">{{ title }}</h3>
        <p v-if="subtitle" class="text-sm text-text-secondary">{{ subtitle }}</p>
      </div>
      <div v-if="showTotal" class="flex items-center gap-2 text-sm text-text-tertiary">
        <span class="text-xs uppercase tracking-wide">Всего</span>
        <AppBadge variant="info">{{ permissionCount }}</AppBadge>
      </div>
    </div>

    <div :class="contentClass">
      <div v-for="domain in groupedKeys" :key="domain" :class="domainCardClass(domain)">
        <div :class="domainHeaderClass(domain)">
          <div>
            <p class="text-xs uppercase tracking-widest text-text-tertiary">Домен</p>
            <p class="text-base font-semibold text-text-primary">{{ domain }}</p>
            <p v-if="domainHint" class="text-xs text-text-secondary mt-0.5">{{ domainHint }}</p>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <AppBadge variant="secondary">
              {{ groupedPermissions[domain].length }}
              {{ groupedPermissions[domain].length === 1 ? 'право' : 'права' }}
            </AppBadge>
            <AppBadge :variant="domainSelectionBadgeVariant(domain)">
              {{ selectedCount(domain) }}/{{ groupedPermissions[domain].length }} выбрано
            </AppBadge>
            <AppButton
              v-if="isEditable"
              type="button"
              :variant="domainActionVariant(domain)"
              size="xs"
              @click="toggleDomain(domain)"
            >
              {{ isDomainSelected(domain) ? 'Снять все' : 'Выбрать все' }}
            </AppButton>
          </div>
        </div>
        <div class="grid gap-px bg-border-secondary/60 sm:grid-cols-2">
          <template v-if="isEditable">
            <label
              v-for="perm in groupedPermissions[domain]"
              :key="perm.key"
              :class="permissionRowClass(perm.key)"
            >
              <Checkbox
                :model-value="isPermissionSelected(perm.key)"
                class="mt-0.5 h-5 w-5 rounded-md border-primary/40 shadow-none transition data-[state=checked]:border-success data-[state=checked]:bg-success data-[state=checked]:text-white"
                @update:model-value="togglePermission(perm.key)"
              />
              <span class="min-w-0 flex-1">
                <span
                  :class="[
                    'block text-sm font-semibold transition-colors',
                    isPermissionSelected(perm.key) ? 'text-success' : 'text-text-primary',
                  ]"
                >
                  {{ perm.action }}
                </span>
                <span
                  :class="[
                    'block text-xs transition-colors',
                    isPermissionSelected(perm.key) ? 'text-text-primary' : 'text-text-secondary',
                  ]"
                >
                  {{ perm.description || 'Описание отсутствует' }}
                </span>
              </span>
              <AppBadge
                :variant="isPermissionSelected(perm.key) ? 'success' : 'outline'"
                class="self-start whitespace-nowrap"
              >
                {{ isPermissionSelected(perm.key) ? 'Активно' : 'Не выбрано' }}
              </AppBadge>
            </label>
          </template>
          <template v-else>
            <div
              v-for="perm in groupedPermissions[domain]"
              :key="perm.key"
              :class="permissionRowClass(perm.key)"
            >
              <span class="min-w-0 flex-1">
                <span
                  :class="[
                    'block text-sm font-semibold transition-colors',
                    isPermissionSelected(perm.key) ? 'text-success' : 'text-text-primary',
                  ]"
                >
                  {{ perm.action }}
                </span>
                <span
                  :class="[
                    'block text-xs transition-colors',
                    isPermissionSelected(perm.key) ? 'text-text-primary' : 'text-text-secondary',
                  ]"
                >
                  {{ perm.description || 'Описание отсутствует' }}
                </span>
              </span>
              <AppBadge
                :variant="isPermissionSelected(perm.key) ? 'success' : 'outline'"
                class="self-start whitespace-nowrap"
              >
                {{ isPermissionSelected(perm.key) ? 'Назначено' : 'Не выбрано' }}
              </AppBadge>
            </div>
          </template>
        </div>
      </div>
    </div>
  </section>
  <p v-else class="text-sm text-text-secondary">{{ emptyText }}</p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Checkbox } from '@/components/ui/checkbox'

interface PermissionItem {
  domain: string
  action: string
  description?: string | null
  key?: string
}

const props = defineProps({
  permissions: {
    type: Array as () => PermissionItem[],
    default: () => [],
  },
  modelValue: {
    type: Array as () => string[],
    default: undefined,
  },
  editable: {
    type: Boolean,
    default: undefined,
  },
  variant: {
    type: String,
    default: 'inline',
  },
  title: {
    type: String,
    default: 'Права доступа',
  },
  subtitle: {
    type: String,
    default: '',
  },
  headerEyebrow: {
    type: String,
    default: '',
  },
  domainHint: {
    type: String,
    default: 'Выберите действия для домена',
  },
  emptyText: {
    type: String,
    default: 'Права не назначены.',
  },
  showTotal: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const isEditable = computed(() =>
  typeof props.editable === 'boolean' ? props.editable : Array.isArray(props.modelValue),
)

const fallbackSelectedKeys = computed(() =>
  props.permissions.map((perm) => perm.key || `${perm.domain}:${perm.action}`),
)

const selectedKeys = computed(() =>
  Array.isArray(props.modelValue) ? props.modelValue : fallbackSelectedKeys.value,
)

const setSelectedKeys = (value: string[]) => {
  if (!isEditable.value) return
  emit('update:modelValue', value)
}

const groupedPermissions = computed(() => {
  const groups: Record<string, { key: string; action: string; description?: string | null }[]> = {}
  props.permissions.forEach((perm) => {
    const domain = perm.domain || 'default'
    if (!groups[domain]) {
      groups[domain] = []
    }
    const key = perm.key || `${perm.domain}:${perm.action}`
    groups[domain].push({
      key,
      action: perm.action,
      description: perm.description,
    })
  })

  Object.keys(groups).forEach((domain) => {
    groups[domain].sort((a, b) => a.action.localeCompare(b.action))
  })

  return groups
})

const groupedKeys = computed(() => Object.keys(groupedPermissions.value).sort())
const permissionCount = computed(() =>
  Object.values(groupedPermissions.value).reduce((total, perms) => total + perms.length, 0),
)

const togglePermission = (key: string) => {
  if (!isEditable.value) return
  if (selectedKeys.value.includes(key)) {
    setSelectedKeys(selectedKeys.value.filter((item) => item !== key))
  } else {
    setSelectedKeys([...selectedKeys.value, key])
  }
}

const isDomainSelected = (domain: string) => {
  const domainPerms = groupedPermissions.value[domain] || []
  return domainPerms.every((perm) => selectedKeys.value.includes(perm.key))
}

const isPermissionSelected = (key: string) => selectedKeys.value.includes(key)

const selectedCount = (domain: string) => {
  const domainPerms = groupedPermissions.value[domain] || []
  return domainPerms.filter((perm) => selectedKeys.value.includes(perm.key)).length
}

const isDomainPartiallySelected = (domain: string) => {
  const count = selectedCount(domain)
  return count > 0 && count < (groupedPermissions.value[domain] || []).length
}

const toggleDomain = (domain: string) => {
  if (!isEditable.value) return
  const domainPerms = groupedPermissions.value[domain] || []
  if (isDomainSelected(domain)) {
    setSelectedKeys(selectedKeys.value.filter((perm) => !domainPerms.some((p) => p.key === perm)))
  } else {
    const keys = domainPerms.map((perm) => perm.key)
    const merged = new Set([...selectedKeys.value, ...keys])
    setSelectedKeys(Array.from(merged))
  }
}

const domainSelectionBadgeVariant = (domain: string) => {
  if (isDomainSelected(domain)) return 'success'
  if (isDomainPartiallySelected(domain)) return 'warning'
  return 'outline'
}

const domainActionVariant = (domain: string) => {
  if (isDomainSelected(domain)) return 'secondary'
  if (isDomainPartiallySelected(domain)) return 'warning'
  return 'outline'
}

const domainCardClass = (domain: string) => {
  if (isDomainSelected(domain)) {
    return 'border border-success/35 rounded-2xl overflow-hidden bg-success/5 shadow-sm'
  }
  if (isDomainPartiallySelected(domain)) {
    return 'border border-warning/35 rounded-2xl overflow-hidden bg-warning/5 shadow-sm'
  }
  return 'border border-border-secondary rounded-2xl overflow-hidden bg-surface'
}

const domainHeaderClass = (domain: string) => {
  if (isDomainSelected(domain)) {
    return 'flex flex-wrap items-center justify-between gap-3 px-4 py-3 border-b border-success/20 bg-success/10'
  }
  if (isDomainPartiallySelected(domain)) {
    return 'flex flex-wrap items-center justify-between gap-3 px-4 py-3 border-b border-warning/20 bg-warning/10'
  }
  return 'flex flex-wrap items-center justify-between gap-3 px-4 py-3 border-b border-border-secondary bg-surface-variant/60'
}

const permissionRowClass = (key: string) => {
  if (!isEditable.value) {
    if (isPermissionSelected(key)) {
      return 'flex items-start gap-3 bg-success/10 px-4 py-3 transition-colors border-l-2 border-l-success'
    }
    return 'flex items-start gap-3 bg-card px-4 py-3 transition-colors border-l-2 border-l-transparent'
  }

  if (isPermissionSelected(key)) {
    return 'flex items-start gap-3 bg-success/10 px-4 py-3 cursor-pointer select-none transition-colors border-l-2 border-l-success'
  }
  return 'flex items-start gap-3 bg-card px-4 py-3 cursor-pointer select-none transition-colors border-l-2 border-l-transparent hover:bg-surface-variant/40'
}

const wrapperClass = computed(() =>
  props.variant === 'card'
    ? 'bg-card border border-border-primary shadow-sm sm:rounded-2xl overflow-hidden'
    : 'space-y-4',
)

const headerClass = computed(() =>
  props.variant === 'card'
    ? 'px-5 py-4 border-b border-border-secondary bg-surface-variant/40 flex flex-wrap items-center justify-between gap-3'
    : 'flex flex-wrap items-center justify-between gap-3',
)

const contentClass = computed(() =>
  props.variant === 'card' ? 'px-5 py-5 space-y-5' : 'space-y-4',
)
const showHeader = computed(() => Boolean(props.title || props.subtitle || props.headerEyebrow))
</script>
