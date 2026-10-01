<template>
  <AppHead :title="props.title" />
  <PageActionBar :back-href="basePath" label="Просмотр" />

  <div class="max-w-4xl mx-auto space-y-8">
    <Card class="overflow-hidden">
      <div class="px-6 py-8 sm:px-8">
        <div class="flex flex-col sm:flex-row sm:items-center gap-6">
          <div class="flex-shrink-0">
            <div
              class="h-24 w-24 bg-gradient-to-br from-primary to-primary-light rounded-full flex items-center justify-center shadow-lg"
            >
              <span class="text-3xl font-bold text-primary-contrast">{{ userInitials }}</span>
            </div>
          </div>

          <div class="flex-grow">
            <div class="flex items-center gap-3 mb-2">
              <h1 class="text-2xl font-bold text-text-primary">{{ fullName }}</h1>
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
      <Card>
        <CardHeader class="border-b border-border-primary">
          <div class="flex items-center">
            <Mail class="h-5 w-5 text-primary mr-2" />
            <h2 class="text-lg font-semibold text-text-primary">Контакты</h2>
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
            <span class="text-text-secondary font-medium">Телефон</span>
            <div class="flex items-center">
              <span class="text-text-primary">{{ props.data.phone ?? '—' }}</span>
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader class="border-b border-border-primary">
          <div class="flex items-center">
            <ShieldCheck class="h-5 w-5 text-primary mr-2" />
            <h2 class="text-lg font-semibold text-text-primary">Подтверждения</h2>
          </div>
        </CardHeader>
        <CardContent class="space-y-4 pt-6">
          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Email</span>
            <span
              class="text-sm"
              :class="props.data.confirmedEmailAt?.Valid ? 'text-success' : 'text-error'"
            >
              {{ props.data.confirmedEmailAt?.Valid ? 'Подтвержден' : 'Не подтвержден' }}
            </span>
          </div>
          <div class="flex items-center justify-between">
            <span class="text-text-secondary font-medium">Телефон</span>
            <span
              class="text-sm"
              :class="props.data.confirmedPhoneAt?.Valid ? 'text-success' : 'text-error'"
            >
              {{ props.data.confirmedPhoneAt?.Valid ? 'Подтвержден' : 'Не подтвержден' }}
            </span>
          </div>
        </CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader class="border-b border-border-primary">
        <div class="flex items-center">
          <Clock class="h-5 w-5 text-primary mr-2" />
          <h2 class="text-lg font-semibold text-text-primary">История</h2>
        </div>
      </CardHeader>
      <CardContent class="grid grid-cols-1 md:grid-cols-2 gap-6 pt-6">
        <div>
          <p class="text-sm font-medium text-text-primary">Создан</p>
          <p class="text-sm text-text-secondary">{{ formatDateTime(props.data.createdAt) }}</p>
        </div>
        <div>
          <p class="text-sm font-medium text-text-primary">Обновлен</p>
          <p class="text-sm text-text-secondary">{{ formatDateTime(props.data.updatedAt) }}</p>
        </div>
      </CardContent>
    </Card>

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
import AppButton from '@/components/ui/AppButton.vue'
import { Link } from '@inertiajs/vue3'
import { ArrowLeft, Calendar, Clock, Hash, Mail, Pencil, ShieldCheck, User } from 'lucide-vue-next'
import { Card, CardHeader, CardContent } from '@/components/ui/card'

const basePath = '/users'
const props = defineProps({
  title: { type: [String], default: undefined },
  data: { type: Object, default: () => ({}) },
})

const fullName = computed(
  () => `${props.data.name || ''} ${props.data.lastName || ''}`.trim() || 'Без имени',
)
const userInitials = computed(() => {
  const name = props.data.name || ''
  const lastName = props.data.lastName || ''
  return name.charAt(0).toUpperCase() + lastName.charAt(0).toUpperCase() || 'U'
})

const formatDate = (s: string) =>
  s
    ? new Date(s).toLocaleDateString('ru-RU', { year: 'numeric', month: 'long', day: 'numeric' })
    : '—'
const formatDateTime = (s: string) =>
  s
    ? new Date(s).toLocaleString('ru-RU', {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    : '—'
</script>
