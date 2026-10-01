<template>
  <AppHead :title="pageTitle" />

  <p v-if="!notificationsEnabled" class="text-text-secondary">Уведомления недоступны.</p>

  <Card
    v-if="notificationsEnabled"
    class="overflow-hidden border-border-primary bg-gradient-to-br from-card via-surface to-surface-variant"
  >
    <div
      class="p-6 flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between border-b border-border-secondary"
    >
      <div>
        <h1 class="text-xl font-semibold text-text-primary">Центр уведомлений</h1>
        <p class="text-sm text-text-secondary">Все события, адресованные вам, собираются здесь.</p>
      </div>
      <div class="flex flex-col sm:flex-row gap-3 w-full sm:w-auto">
        <div class="grid grid-cols-2 gap-3 sm:flex sm:items-center">
          <div
            class="text-center px-3 py-2 rounded-lg bg-surface/70 border border-border-secondary min-w-[96px]"
          >
            <p class="text-xs uppercase text-text-tertiary">Всего</p>
            <p class="text-lg font-semibold text-text-primary">{{ meta.total }}</p>
          </div>
          <div
            class="text-center px-3 py-2 rounded-lg border min-w-[120px]"
            :class="
              unreadCount > 0
                ? 'bg-warning/10 border-warning/50'
                : 'bg-surface/70 border-border-secondary'
            "
          >
            <p class="text-xs uppercase text-text-tertiary">Непрочитанных</p>
            <p
              class="text-lg font-semibold"
              :class="unreadCount > 0 ? 'text-warning' : 'text-text-primary'"
            >
              {{ unreadCount }}
            </p>
          </div>
        </div>
        <AppButton
          :variant="unreadCount > 0 ? 'primary' : 'secondary'"
          size="sm"
          type="button"
          :disabled="unreadCount === 0 || markAllForm.processing"
          @click="markAllRead"
        >
          {{ markAllForm.processing ? 'Отмечаем…' : 'Прочитать всё' }}
        </AppButton>
      </div>
    </div>

    <div
      class="p-5 border-b border-border-secondary flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between bg-card"
    >
      <div class="flex gap-2 flex-wrap items-center">
        <AppButton
          v-for="option in statusOptions"
          :key="option.value"
          :variant="filterForm.status === option.value ? 'secondary' : 'ghost'"
          size="xs"
          type="button"
          @click="changeStatus(option.value)"
        >
          {{ option.label }}
          <AppBadge
            v-if="option.value === 'unread' && unreadCount > 0"
            variant="primary"
            class="ml-1 text-[11px]"
          >
            {{ unreadCount }}
          </AppBadge>
        </AppButton>
      </div>
      <div class="flex items-center gap-2 text-sm text-text-secondary">
        <label for="perPage" class="text-xs uppercase tracking-wide text-text-tertiary"
          >На странице</label
        >
        <AppSelect
          id="perPage"
          :model-value="filterForm.perPage"
          :options="perPageOptions.map((option) => ({ label: String(option), value: option }))"
          size="sm"
          @update:modelValue="(value) => updatePerPage(value)"
        />
      </div>
    </div>

    <div class="p-5">
      <template v-if="hasNotifications">
        <div
          class="divide-y divide-border-secondary border border-border-secondary rounded-2xl overflow-hidden bg-card"
        >
          <article
            v-for="notification in notifications"
            :key="notification.id"
            class="flex flex-col gap-3 px-5 py-4 lg:flex-row lg:items-center"
            :class="[
              notification.isRead ? 'bg-card/70' : 'bg-surface',
              notificationTone(notification.level),
            ]"
          >
            <div class="flex items-start gap-3 flex-1">
              <div class="flex flex-col items-center gap-2 pt-1">
                <span
                  class="inline-flex h-2.5 w-2.5 rounded-full"
                  :class="notification.isRead ? 'bg-border-secondary' : 'bg-primary'"
                  aria-hidden="true"
                />
                <span
                  class="hidden lg:inline-flex h-full w-px bg-border-secondary/60"
                  aria-hidden="true"
                />
              </div>
              <div class="space-y-1 w-full">
                <div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                  <div class="flex items-center gap-2 flex-wrap">
                    <p class="font-semibold text-text-primary">
                      {{ notification.title }}
                    </p>
                    <AppBadge
                      :variant="levelVariant(notification.level).variant"
                      class="text-[11px] uppercase tracking-wide"
                    >
                      {{ levelVariant(notification.level).label }}
                    </AppBadge>
                  </div>
                  <p class="text-xs text-text-tertiary">
                    {{ formatDateTime(notification.createdAt) }}
                  </p>
                </div>
                <p class="text-sm text-text-secondary whitespace-pre-line">
                  {{ notification.message }}
                </p>
                <div v-if="hasPayloadLink(notification.payload)" class="pt-1">
                  <a
                    :href="String(notification.payload?.url)"
                    target="_blank"
                    rel="noopener noreferrer"
                    class="inline-flex items-center gap-1 text-sm text-primary hover:underline font-medium"
                  >
                    {{ notification.payload?.ctaText ?? 'Перейти' }}
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="1.5"
                        d="M7.5 16.5 16.5 7.5M9 7.5h7.5V15"
                      />
                    </svg>
                  </a>
                </div>
                <div
                  v-if="payloadEntries(notification.payload).length"
                  class="pt-2 flex flex-wrap gap-2"
                >
                  <span
                    v-for="[key, value] in payloadEntries(notification.payload)"
                    :key="`${notification.id}-${key}`"
                    class="px-2 py-0.5 rounded-full bg-surface-variant/60 text-xs text-text-tertiary"
                  >
                    <span class="font-semibold text-text-secondary">{{ key }}:</span>
                    <span class="ml-1">{{ formatPayloadValue(value) }}</span>
                  </span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-2">
              <AppButton
                :variant="notification.isRead ? 'ghost' : 'secondary'"
                size="sm"
                type="button"
                :disabled="notification.isRead || markReadForm.processing"
                @click="markNotification(notification.id)"
              >
                {{
                  pendingNotificationId === notification.id
                    ? '...'
                    : notification.isRead
                      ? 'Прочитано'
                      : 'Отметить'
                }}
              </AppButton>
            </div>
          </article>
        </div>
      </template>
      <div v-else class="py-16 text-center space-y-3">
        <p class="text-lg font-semibold text-text-primary">Нет уведомлений</p>
        <p class="text-sm text-text-secondary">
          Когда появятся события для вас, они отобразятся здесь.
        </p>
      </div>
    </div>

    <div
      v-if="meta.totalPages > 1"
      class="px-5 pb-5 flex items-center justify-between flex-wrap gap-3 border-t border-border-secondary pt-4"
    >
      <p class="text-sm text-text-secondary">Страница {{ meta.page }} из {{ meta.totalPages }}</p>
      <div class="flex items-center gap-2">
        <AppButton
          variant="secondary"
          size="sm"
          :disabled="meta.page <= 1"
          @click="goToPage(meta.page - 1)"
        >
          Назад
        </AppButton>
        <AppButton
          variant="secondary"
          size="sm"
          :disabled="meta.page >= meta.totalPages"
          @click="goToPage(meta.page + 1)"
        >
          Вперёд
        </AppButton>
      </div>
    </div>
  </Card>
</template>

<script setup lang="ts">
import { computed, type PropType, ref } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { router, useForm } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import type { AdminNotification } from '@/types/models'
import { Card } from '@/components/ui/card'

const { notifications: notificationsEnabled } = useAdminCapabilities()

type Meta = {
  total: number
  unreadCount: number
  page: number
  perPage: number
  totalPages: number
  hasMore: boolean
}

type Filters = {
  status?: string
  page?: number
  perPage?: number
}

const props = defineProps({
  title: {
    type: String,
    default: 'Уведомления',
  },
  notifications: {
    type: Array as PropType<AdminNotification[]>,
    default: () => [],
  },
  meta: {
    type: Object as PropType<Meta>,
    default: () => ({
      total: 0,
      unreadCount: 0,
      page: 1,
      perPage: 10,
      totalPages: 1,
      hasMore: false,
    }),
  },
  filters: {
    type: Object as PropType<Filters>,
    default: () => ({
      status: 'all',
      page: 1,
      perPage: 10,
    }),
  },
})

const pageTitle = computed(() => props.title || 'Уведомления')
const meta = computed<Meta>(
  () =>
    props.meta ?? { total: 0, unreadCount: 0, page: 1, perPage: 10, totalPages: 1, hasMore: false },
)
const notifications = computed<AdminNotification[]>(() => props.notifications ?? [])
const unreadCount = computed(() => props.meta?.unreadCount ?? 0)
const hasNotifications = computed(() => notifications.value.length > 0)

const statusOptions = [
  { value: 'all', label: 'Все' },
  { value: 'unread', label: 'Непрочитанные' },
]
const perPageOptions = [5, 10, 20, 30, 50]

const filterForm = useForm({
  status: props.filters?.status ?? 'all',
  perPage: props.filters?.perPage ?? 10,
})

const markReadForm = useForm({})
const markAllForm = useForm({})
const pendingNotificationId = ref<number | null>(null)

const changeStatus = (status: string) => {
  if (filterForm.status === status) {
    return
  }

  filterForm.status = status
  applyFilters({ page: 1 })
}

const updatePerPage = (value: unknown) => {
  filterForm.perPage = Number(value)
  applyFilters({ page: 1 })
}

const applyFilters = (extra: Record<string, number> = {}) => {
  if (!notificationsEnabled.value) return
  router.get(
    '/notifications',
    {
      status: filterForm.status,
      perPage: filterForm.perPage,
      page: extra.page ?? meta.value.page ?? 1,
    },
    {
      preserveScroll: true,
      preserveState: true,
    },
  )
}

const goToPage = (page: number) => {
  if (page === meta.value.page || page < 1 || page > (meta.value.totalPages ?? 1)) {
    return
  }
  applyFilters({ page })
}

const markNotification = (notificationId: number) => {
  if (!notificationsEnabled.value || markReadForm.processing) {
    return
  }
  pendingNotificationId.value = notificationId
  markReadForm.post(`/notifications/${notificationId}/read`, {
    preserveScroll: true,
    preserveState: true,
    onFinish: () => {
      pendingNotificationId.value = null
    },
  })
}

const markAllRead = () => {
  if (!notificationsEnabled.value || markAllForm.processing || unreadCount.value === 0) {
    return
  }
  markAllForm.post('/notifications/read-all', {
    preserveScroll: true,
    preserveState: true,
  })
}

const formatDateTime = (dateString: string) => {
  const date = new Date(dateString)
  if (Number.isNaN(date.getTime())) {
    return dateString
  }
  return date.toLocaleString('ru-RU', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const levelVariant = (level: string) => {
  const normalized = level?.toLowerCase()
  const variants: Record<
    string,
    { label: string; variant: 'success' | 'warning' | 'danger' | 'info' }
  > = {
    success: { label: 'Успех', variant: 'success' },
    warning: { label: 'Важно', variant: 'warning' },
    error: { label: 'Ошибка', variant: 'danger' },
    danger: { label: 'Ошибка', variant: 'danger' },
    info: { label: 'Инфо', variant: 'info' },
  }

  return variants[normalized] ?? { label: 'Инфо', variant: 'info' }
}

const notificationTone = (level: string) => {
  const normalized = level?.toLowerCase()
  const tones: Record<string, string> = {
    success: 'border-l-4 border-success/60',
    warning: 'border-l-4 border-warning/70',
    error: 'border-l-4 border-destructive/70',
    danger: 'border-l-4 border-destructive/70',
    info: 'border-l-4 border-info/60',
  }
  return tones[normalized] ?? 'border-l-4 border-primary/50'
}

const hasPayloadLink = (payload?: Record<string, unknown> | null) => {
  if (!payload) return false
  const url = payload.url ?? payload.link ?? payload.href
  return typeof url === 'string' && url.length > 0
}

const payloadEntries = (payload?: Record<string, unknown> | null) => {
  if (!payload) return []
  return Object.entries(payload).filter(
    ([key]) => !['url', 'href', 'link', 'ctaText'].includes(key),
  )
}

const formatPayloadValue = (value: unknown) => {
  if (value == null) return '—'
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}
</script>
