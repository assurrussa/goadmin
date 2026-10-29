<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import AppStatus from '@/components/ui/AppStatus.vue'
import { formatDateTime, locale, t } from '@/i18n'
import { dashboardText, type DashboardDisplayCard } from './useDashboard'

defineProps<{ card: DashboardDisplayCard }>()
const variants = {
  neutral: 'secondary',
  info: 'info',
  success: 'success',
  warning: 'warning',
  error: 'danger',
} as const
</script>

<template>
  <section
    :data-dashboard-card="card.key"
    class="min-w-0 space-y-4 rounded-lg border border-border-primary bg-card p-5"
  >
    <h2 class="break-words text-lg font-semibold text-text-primary">
      {{ dashboardText(card.title, locale) }}
    </h2>
    <p v-if="card.state === 'loading'" class="text-sm text-text-secondary" role="status">
      {{ t('Загрузка показателей…') }}
    </p>
    <dl v-if="card.metrics.length" class="space-y-3">
      <div v-for="metric in card.metrics" :key="metric.key" class="min-w-0">
        <dt class="break-words text-sm text-text-secondary">
          {{ dashboardText(metric.label, locale) }}
        </dt>
        <dd
          :data-dashboard-metric="metric.key"
          class="break-words text-2xl font-semibold text-text-primary"
        >
          {{ metric.value }}
        </dd>
      </div>
    </dl>
    <AppStatus v-if="card.state === 'unavailable'" variant="warning" class="max-w-full">
      <span class="min-w-0 break-words">{{
        card.hasSuccessfulData
          ? t('Не удалось обновить. Показаны последние полученные данные.')
          : t('Показатели временно недоступны.')
      }}</span>
    </AppStatus>
    <AppStatus
      v-if="card.status && (card.state === 'ready' || card.hasSuccessfulData)"
      :variant="card.state === 'unavailable' ? 'secondary' : variants[card.status.tone]"
      class="max-w-full"
    >
      <span class="min-w-0 break-words">{{ dashboardText(card.status.text, locale) }}</span>
    </AppStatus>
    <p v-if="card.updatedAt" class="break-words text-xs text-text-secondary">
      {{ t('Обновлено') }}:
      <time :datetime="card.updatedAt">{{ formatDateTime(card.updatedAt) }}</time>
    </p>
    <ul v-if="card.links.length" class="space-y-2">
      <li v-for="link in card.links" :key="link.href" class="min-w-0 break-words">
        <Link
          :href="link.href"
          class="text-sm text-primary hover:underline focus-visible:underline"
        >
          {{ dashboardText(link.label, locale) }}
        </Link>
      </li>
    </ul>
  </section>
</template>
