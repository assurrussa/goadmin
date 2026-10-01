<template>
  <AppHead :title="props.title" />

  <div class="max-w-4xl mx-auto space-y-8">
    <!-- Заголовок с основной информацией -->
    <Card class="overflow-hidden">
      <div class="px-6 py-8 sm:px-8">
        <div class="flex flex-col sm:flex-row sm:items-center gap-6">
          <!-- Аватар -->
          <div class="flex-shrink-0 h-24 w-24">
            <div
              class="relative h-24 w-24 rounded-full overflow-hidden shadow-lg bg-gradient-to-br from-primary to-primary-light flex items-center justify-center"
            >
              <img
                v-if="avatarUrl"
                :src="avatarUrl"
                alt="Avatar"
                class="h-full w-full object-cover"
              />
              <span v-else class="text-3xl font-bold text-primary-contrast">
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
              <AppBadge variant="success" class="gap-1.5">
                <span class="w-1.5 h-1.5 bg-success rounded-full"></span>
                Активен
              </AppBadge>
              <AppBadge v-if="primaryRoleLabel" variant="primary" class="gap-1">
                <ShieldCheck class="h-3.5 w-3.5" />
                {{ primaryRoleLabel }}
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
              <Link :href="`${basePath}/edit`">
                <Pencil class="h-4 w-4 mr-2" />
                Редактировать
              </Link>
            </AppButton>
          </div>
        </div>
      </div>
    </Card>

    <Card v-if="uploads">
      <CardHeader
        class="border-b border-border-primary flex flex-row items-center justify-between space-y-0 pb-4"
      >
        <div class="flex items-center">
          <Pencil class="h-5 w-5 text-primary mr-2" />
          <h2 class="text-lg font-semibold text-text-primary">Обновление аватара</h2>
        </div>
        <span class="text-sm text-text-tertiary">Поддерживаются изображения до 5MB</span>
      </CardHeader>
      <CardContent class="pt-6">
        <FormImageUploader
          :model-value="avatarFile"
          label="Аватар администратора"
          description="Загрузите квадратное изображение высокого качества"
          object-type="admin"
          :object-id="props.data.id"
          :upload-url="avatarUploadUrl"
          :delete-url="avatarDeleteUrl"
          :max-file-size="5"
          context="avatar"
          :accepted-file-types="['image/jpeg', 'image/png', 'image/gif', 'image/webp']"
          :enable-cropper="true"
          :cropper-default-aspect-ratio="1"
          :cropper-aspect-ratio-options="avatarAspectRatioOptions"
          :cropper-max-width="512"
          :cropper-max-height="512"
          @update:model-value="handleAvatarUpdate"
          @deleted="handleAvatarDeleted"
        />
      </CardContent>
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
            <div class="flex flex-wrap gap-2 justify-end max-w-xs">
              <AppBadge v-for="role in formattedRoles" :key="role" variant="primary" class="gap-1">
                <ShieldCheck class="h-3 w-3" />
                {{ role }}
              </AppBadge>
              <span v-if="formattedRoles.length === 0" class="text-xs text-text-tertiary">
                Роль не назначена
              </span>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Статус</span>
            <AppBadge variant="success" class="gap-1.5">
              <span class="w-1.5 h-1.5 bg-success rounded-full"></span>
              Активен
            </AppBadge>
          </div>

          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Уникальный ID</span>
            <div class="flex items-center">
              <Hash class="h-4 w-4 text-text-tertiary mr-2" />
              <span class="text-text-primary font-mono">{{ props.data.id }}</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>

    <!-- Временные метки -->
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
      <AppButton asChild>
        <Link :href="`${basePath}/edit`">
          <Pencil class="h-4 w-4 mr-2" />
          Редактировать
        </Link>
      </AppButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, type PropType } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import AppHead from '@/components/layout/AppHead.vue'
import { Link } from '@inertiajs/vue3'
import { useAdminProfileStore } from '@/stores/adminProfile'
import { FormImageUploader } from '@/components/form'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import type { UploadedFile } from '@/components/form/ImageUploader.vue'
import type { CropperAspectRatioOption } from '@/components/form/ImageUploader.vue'
import {
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

const { uploads } = useAdminCapabilities()
const basePath = '/auth/profile'
interface ImagePreview {
  id: number
  url: string
  fileName?: string
  filename?: string
  originalName?: string
  originalFilename?: string
}

interface ProfileData {
  id: number
  name: string
  lastName: string
  email: string
  username: string
  preview?: ImagePreview
  previewUrl?: string
  createdAt: string
  updatedAt: string
  deletedAt?: { Valid: boolean; Time: string }
  data?: { roles?: string[] }
}

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  data: {
    type: Object as PropType<ProfileData>,
    default: () => ({}),
  },
  admin_preview: {
    type: Object as PropType<ImagePreview | null>,
    default: () => null,
  },
})

const profileStore = useAdminProfileStore()

const getPreview = (): ImagePreview | null => props.admin_preview ?? props.data.preview ?? null

const getPreviewUrl = (): string | null =>
  props.admin_preview?.url ?? props.data.previewUrl ?? props.data.preview?.url ?? null

profileStore.ensureAvatarUrl(getPreviewUrl())

const avatarAspectRatioOptions: CropperAspectRatioOption[] = [
  { value: '1', label: '1:1 (квадрат)' },
]

const avatarUrl = computed({
  get: () => profileStore.avatarUrl,
  set: (value: string | null) => profileStore.setAvatarUrl(value),
})

const initialAvatarFile = (): UploadedFile | null => {
  const url = getPreviewUrl()
  if (!url) {
    return null
  }

  const preview = getPreview()

  return {
    id: preview?.id ?? 0,
    url,
    fileName: preview?.fileName ?? preview?.filename ?? '',
    originalName:
      preview?.originalName ??
      preview?.originalFilename ??
      preview?.fileName ??
      preview?.filename ??
      '',
  }
}

const avatarFile = ref<UploadedFile | null>(initialAvatarFile())

const handleAvatarUpdate = (file: UploadedFile | null) => {
  avatarFile.value = file
  profileStore.setAvatarUrl(file?.url ?? null)
}

const handleAvatarDeleted = () => {
  avatarFile.value = null
  profileStore.setAvatarUrl(null)
}

const avatarUploadUrl = '/auth/profile/avatar'
const avatarDeleteUrl = '/auth/profile/avatar'

watch(
  () => getPreviewUrl(),
  (previewUrl) => {
    profileStore.ensureAvatarUrl(previewUrl)
    if (previewUrl) {
      const preview = getPreview()
      avatarFile.value = {
        id: preview?.id ?? avatarFile.value?.id ?? 0,
        url: previewUrl,
        fileName: preview?.fileName ?? preview?.filename ?? avatarFile.value?.fileName ?? '',
        originalName:
          preview?.originalName ??
          preview?.originalFilename ??
          preview?.fileName ??
          preview?.filename ??
          avatarFile.value?.originalName ??
          '',
      }
    } else if (!previewUrl) {
      avatarFile.value = null
    }
  },
)

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

const normalizeRoles = (): string[] => {
  const roles = props.data?.data?.roles as string[] | string | undefined
  if (Array.isArray(roles)) {
    return roles.filter((role) => typeof role === 'string' && role.length > 0)
  }
  if (typeof roles === 'string' && roles.length > 0) {
    return [roles]
  }
  return []
}

const formatRoleLabel = (role: string): string => {
  return role
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

const assignedRoles = computed(() => normalizeRoles())
const formattedRoles = computed(() => assignedRoles.value.map((role) => formatRoleLabel(role)))
const primaryRoleLabel = computed(() => formattedRoles.value[0] ?? '')

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
