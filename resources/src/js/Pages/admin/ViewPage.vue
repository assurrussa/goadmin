<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" label="Просмотр" />

  <div class="max-w-4xl mx-auto space-y-8">
    <!-- Заголовок с основной информацией -->
    <Card class="overflow-hidden">
      <div class="px-6 py-8 sm:px-8">
        <div class="flex flex-col sm:flex-row sm:items-center gap-6">
          <!-- Аватар -->
          <div class="flex-shrink-0">
            <div
              class="h-24 w-24 bg-gradient-to-br from-primary to-primary-light rounded-full flex items-center justify-center shadow-lg"
            >
              <span class="text-3xl font-bold text-primary-contrast">
                {{ userInitials }}
              </span>
            </div>
          </div>

          <!-- Основная информация -->
          <div class="flex-grow">
            <div class="flex items-center gap-3 mb-2">
              <h1 class="text-2xl font-bold text-text-primary">
                {{ fullName }}
              </h1>
              <AppBadge :variant="accountStatusVariant" class="gap-1.5">
                <span
                  class="w-1.5 h-1.5 rounded-full"
                  :class="accountStatusVariant === 'success' ? 'bg-success' : 'bg-error'"
                ></span>
                {{ accountStatusLabel }}
              </AppBadge>
            </div>

            <div class="flex items-center text-text-secondary mb-4">
              <User class="h-4 w-4 mr-2" />
              <span class="font-medium">@{{ props.data.username }}</span>
            </div>

            <div class="flex flex-wrap gap-4 text-sm text-text-secondary">
              <div class="flex items-center">
                <Hash class="h-4 w-4 mr-1.5" />
                <span>ID: {{ props.data.id }}</span>
              </div>
              <div class="flex items-center">
                <Calendar class="h-4 w-4 mr-1.5" />
                <span>Создан {{ formatDate(props.data.createdAt) }}</span>
              </div>
            </div>
          </div>

          <!-- Кнопка редактирования -->
          <div class="flex-shrink-0">
            <AppButton asChild>
              <Link :href="`${basePath}/${props.data.id}/edit`">
                <Pencil class="h-4 w-4 mr-2" />
                Редактировать
              </Link>
            </AppButton>
          </div>
        </div>
      </div>
    </Card>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-8">
      <!-- Контактная информация -->
      <Card>
        <CardHeader class="border-b border-border-primary">
          <div class="flex items-center">
            <Mail class="h-5 w-5 text-primary mr-2" />
            <h2 class="text-lg font-semibold text-text-primary">Контактная информация</h2>
          </div>
        </CardHeader>
        <CardContent class="space-y-4 pt-6">
          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Email</span>
            <div class="flex items-center">
              <Mail class="h-4 w-4 text-text-tertiary mr-2" />
              <span class="text-text-primary">{{ props.data.email }}</span>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Имя пользователя</span>
            <div class="flex items-center">
              <AtSign class="h-4 w-4 text-text-tertiary mr-2" />
              <span class="text-text-primary">{{ props.data.username }}</span>
            </div>
          </div>
        </CardContent>
      </Card>

      <!-- Информация об аккаунте -->
      <Card>
        <CardHeader class="border-b border-border-primary">
          <div class="flex items-center">
            <ShieldCheck class="h-5 w-5 text-primary mr-2" />
            <h2 class="text-lg font-semibold text-text-primary">Информация об аккаунте</h2>
          </div>
        </CardHeader>
        <CardContent class="space-y-4 pt-6">
          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Роль</span>
            <div class="flex flex-wrap justify-end gap-2 max-w-[65%]">
              <AppBadge v-for="role in rolesList" :key="role" variant="primary" class="gap-1">
                <ShieldCheck class="h-3 w-3" />
                {{ role }}
              </AppBadge>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Статус</span>
            <AppBadge :variant="accountStatusVariant" class="gap-1.5">
              <span
                class="w-1.5 h-1.5 rounded-full"
                :class="accountStatusVariant === 'success' ? 'bg-success' : 'bg-error'"
              ></span>
              {{ accountStatusLabel }}
            </AppBadge>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Email</span>
            <AppBadge :variant="emailConfirmed ? 'success' : 'warning'">
              {{ emailConfirmed ? 'Подтверждён' : 'Не подтверждён' }}
            </AppBadge>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Уникальный ID</span>
            <div class="flex items-center">
              <Hash class="h-4 w-4 text-text-tertiary mr-2" />
              <span class="text-text-primary font-mono">{{ props.data.id }}</span>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Последний вход</span>
            <span class="text-text-primary">
              {{ lastLoginAt ? formatDateTime(lastLoginAt) : 'Нет данных' }}
            </span>
          </div>
        </CardContent>
      </Card>
    </div>

    <!-- Временные метки -->
    <Card>
      <CardHeader class="border-b border-border-primary">
        <div class="flex items-center">
          <Clock class="h-5 w-5 text-primary mr-2" />
          <h2 class="text-lg font-semibold text-text-primary">История изменений</h2>
        </div>
      </CardHeader>
      <CardContent class="pt-6">
        <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div class="flex items-start space-x-3">
            <div class="flex-shrink-0">
              <div
                class="w-10 h-10 bg-surface-variant rounded-full flex items-center justify-center"
              >
                <Plus class="h-5 w-5 text-success" />
              </div>
            </div>
            <div class="flex-grow">
              <p class="text-sm font-medium text-text-primary">Создан</p>
              <p class="text-sm text-text-secondary">{{ formatDateTime(props.data.createdAt) }}</p>
              <p class="text-xs text-text-tertiary mt-1">{{ timeAgo(props.data.createdAt) }}</p>
            </div>
          </div>

          <div class="flex items-start space-x-3">
            <div class="flex-shrink-0">
              <div
                class="w-10 h-10 bg-surface-variant rounded-full flex items-center justify-center"
              >
                <Pencil class="h-5 w-5 text-primary" />
              </div>
            </div>
            <div class="flex-grow">
              <p class="text-sm font-medium text-text-primary">Последнее обновление</p>
              <p class="text-sm text-text-secondary">{{ formatDateTime(props.data.updatedAt) }}</p>
              <p class="text-xs text-text-tertiary mt-1">{{ timeAgo(props.data.updatedAt) }}</p>
            </div>
          </div>

          <div v-if="lastLoginAt" class="flex items-start space-x-3">
            <div class="flex-shrink-0">
              <div
                class="w-10 h-10 bg-surface-variant rounded-full flex items-center justify-center"
              >
                <Clock class="h-5 w-5 text-info" />
              </div>
            </div>
            <div class="flex-grow">
              <p class="text-sm font-medium text-text-primary">Последний вход</p>
              <p class="text-sm text-text-secondary">{{ formatDateTime(lastLoginAt) }}</p>
              <p class="text-xs text-text-tertiary mt-1">{{ timeAgo(lastLoginAt) }}</p>
            </div>
          </div>

          <div v-if="emailConfirmedAt" class="flex items-start space-x-3">
            <div class="flex-shrink-0">
              <div
                class="w-10 h-10 bg-surface-variant rounded-full flex items-center justify-center"
              >
                <ShieldCheck class="h-5 w-5 text-success" />
              </div>
            </div>
            <div class="flex-grow">
              <p class="text-sm font-medium text-text-primary">Email подтверждён</p>
              <p class="text-sm text-text-secondary">{{ formatDateTime(emailConfirmedAt) }}</p>
              <p class="text-xs text-text-tertiary mt-1">{{ timeAgo(emailConfirmedAt) }}</p>
            </div>
          </div>
        </div>

        <!-- Статус удаления -->
        <div
          v-if="props.data.deletedAt && props.data.deletedAt.Valid"
          class="mt-6 pt-6 border-t border-border-primary"
        >
          <div class="flex items-start space-x-3">
            <div class="flex-shrink-0">
              <div
                class="w-10 h-10 bg-surface-variant rounded-full flex items-center justify-center"
              >
                <Trash2 class="h-5 w-5 text-error" />
              </div>
            </div>
            <div class="flex-grow">
              <p class="text-sm font-medium text-text-primary">Удален</p>
              <p class="text-sm text-text-secondary">
                {{ formatDateTime(props.data.deletedAt.Time) }}
              </p>
              <p class="text-xs text-text-tertiary mt-1">
                {{ timeAgo(props.data.deletedAt.Time) }}
              </p>
            </div>
          </div>
        </div>
      </CardContent>
    </Card>

    <!-- Действия -->
    <div class="flex justify-end space-x-4">
      <AppButton asChild variant="secondary">
        <Link :href="basePath">
          <ArrowLeft class="h-4 w-4 mr-2" />
          Назад к списку
        </Link>
      </AppButton>
      <AppButton asChild>
        <Link :href="`${basePath}/${props.data.id}/edit`">
          <Pencil class="h-4 w-4 mr-2" />
          Редактировать
        </Link>
      </AppButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Link } from '@inertiajs/vue3'
import {
  ArrowLeft,
  AtSign,
  Calendar,
  Clock,
  Hash,
  Mail,
  Pencil,
  Plus,
  ShieldCheck,
  Trash2,
  User,
} from 'lucide-vue-next'
import { Card, CardHeader, CardContent } from '@/components/ui/card'

const basePath = '/admins'
const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  data: {
    type: Object,
    default: () => ({}),
  },
})

// Вычисляемые свойства
const fullName = computed(() => {
  return `${props.data.name || ''} ${props.data.lastName || ''}`.trim() || 'Без имени'
})

const userInitials = computed(() => {
  const name = props.data.name || ''
  const lastName = props.data.lastName || ''

  const firstInitial = name.charAt(0).toUpperCase()
  const lastInitial = lastName.charAt(0).toUpperCase()

  return firstInitial + lastInitial || 'AD'
})

const rolesList = computed(() => {
  const roles = props.data?.data?.roles
  return Array.isArray(roles) && roles.length > 0 ? roles : ['Без роли']
})

const emailConfirmedAt = computed(() => props.data?.data?.emailConfirmedAt || '')
const lastLoginAt = computed(() => props.data?.data?.lastLoginAt || '')
const emailConfirmed = computed(() => Boolean(emailConfirmedAt.value))
const accountStatusLabel = computed(() => (props.data?.deletedAt?.Valid ? 'Удалён' : 'Активен'))
const accountStatusVariant = computed(() => (props.data?.deletedAt?.Valid ? 'danger' : 'success'))

// Функции форматирования
const formatDate = (dateString: string) => {
  if (!dateString) return 'Не указано'

  const date = new Date(dateString)
  return date.toLocaleDateString('ru-RU', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

const formatDateTime = (dateString: string) => {
  if (!dateString) return 'Не указано'

  const date = new Date(dateString)
  return date.toLocaleString('ru-RU', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const timeAgo = (dateString: string) => {
  if (!dateString) return ''

  const date = new Date(dateString)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffMinutes = Math.floor(diffMs / (1000 * 60))

  if (diffDays > 0) {
    return `${diffDays} ${diffDays === 1 ? 'день' : diffDays < 5 ? 'дня' : 'дней'} назад`
  } else if (diffHours > 0) {
    return `${diffHours} ${diffHours === 1 ? 'час' : diffHours < 5 ? 'часа' : 'часов'} назад`
  } else if (diffMinutes > 0) {
    return `${diffMinutes} ${diffMinutes === 1 ? 'минуту' : diffMinutes < 5 ? 'минуты' : 'минут'} назад`
  } else {
    return 'Только что'
  }
}
</script>
