<template>
  <Card
    v-if="pagination && pagination.totalPages > 1"
    class="px-4 py-3 flex items-center justify-between sm:px-6"
  >
    <!-- Информация о записях (мобильная версия) -->
    <div class="flex-1 flex justify-between sm:hidden">
      <AppButton
        variant="secondary"
        size="sm"
        :disabled="!pagination.prevPageUrl"
        @click="handlePageChange(pagination.prevPageUrl || null)"
      >
        Назад
      </AppButton>
      <AppButton
        variant="secondary"
        size="sm"
        :disabled="!pagination.nextPageUrl"
        @click="handlePageChange(pagination.nextPageUrl || null)"
      >
        Вперед
      </AppButton>
    </div>

    <!-- Десктопная версия -->
    <div class="hidden sm:flex-1 sm:flex sm:items-center sm:justify-between">
      <div class="flex items-center space-x-4">
        <p class="text-sm text-text-secondary">
          Показано
          <span class="font-medium">{{ pagination.from }}</span>
          -
          <span class="font-medium">{{ pagination.to }}</span>
          из
          <span class="font-medium">{{ pagination.total }}</span>
          записей
        </p>
      </div>

      <div class="flex items-center space-x-2">
        <div class="flex items-center space-x-1">
          <template v-for="(link, index) in pagination.links" :key="index">
            <AppButton
              v-if="link.url && link.label !== '...'"
              :variant="getLinkVariant(link)"
              size="sm"
              :disabled="!link.url || link.active"
              @click="handlePageChange(link.url)"
            >
              {{ formatLinkLabel(link.label) }}
            </AppButton>
            <span v-else-if="link.label === '...'" class="px-2 py-1 text-sm text-text-secondary">
              ...
            </span>
            <AppButton v-else variant="secondary" size="sm" disabled>
              {{ formatLinkLabel(link.label) }}
            </AppButton>
          </template>
        </div>
      </div>
    </div>
  </Card>
</template>

<script setup lang="ts">
import type { Pagination, PaginationLink } from '../../composables/useDataGrid.ts'
import { Card } from '@/components/ui/card'
import AppButton from '@/components/ui/AppButton.vue'

// Props interface
interface Props {
  pagination: Pagination | null
}

// Define props with defaults
defineProps<Props>()

// Define emits
const emit = defineEmits<{
  'page-change': [url: string | null]
}>()

// Методы для обработки ссылок
const handlePageChange = (url: string | null): void => {
  if (url) {
    emit('page-change', url)
  }
}

const formatLinkLabel = (label: string): string => {
  // Заменяем "Previous" и "Next" на русские эквиваленты
  if (label === 'Previous') return '‹'
  if (label === 'Next') return '›'
  return label
}

const getLinkVariant = (link: PaginationLink): 'primary' | 'secondary' => {
  if (link.active) return 'primary'
  return 'secondary'
}
</script>
