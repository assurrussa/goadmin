<template>
  <AppHead :title="(title ? t(title) : '') || t('Главная')" />
  <main class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div class="min-w-0">
        <h1 class="text-2xl font-bold text-text-primary">
          {{ (title ? t(title) : '') || t('Главная') }}
        </h1>
        <p v-if="updatedAt" class="mt-1 break-words text-sm text-text-secondary">
          {{ t('Обновлено') }}: <time :datetime="updatedAt">{{ formatDateTime(updatedAt) }}</time>
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-4">
        <AppButton variant="outline" :loading="refreshing" :disabled="stopped" @click="refresh">
          {{ t('Обновить') }}
        </AppButton>
        <label class="flex items-center gap-2 text-sm text-text-secondary">
          <input v-model="autoRefresh" type="checkbox" :disabled="stopped" class="accent-primary" />
          {{ t('Автообновление каждую минуту') }}
        </label>
      </div>
    </div>
    <p v-if="failed" role="status" class="text-sm text-warning">
      {{ t('Не удалось обновить панель. Данные могут быть устаревшими.') }}
    </p>
    <div
      v-if="cards.length"
      class="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3"
      :aria-busy="refreshing"
    >
      <DashboardCard v-for="card in cards" :key="card.key" :card="card" />
    </div>
    <h2 class="text-lg font-semibold text-text-primary">{{ t('Доступные разделы управления') }}</h2>
    <div v-if="sections.length" class="grid gap-4 md:grid-cols-2">
      <section
        v-for="section in sections"
        :key="section.key"
        class="rounded-lg border border-border-primary bg-card p-5"
      >
        <h2 class="mb-3 text-lg font-semibold text-text-primary">
          {{ (section.title ? t(section.title) : '') || t('Разделы') }}
        </h2>
        <ul class="space-y-2">
          <li v-for="item in section.links" :key="item.href">
            <Link :href="item.href" class="text-primary hover:underline focus-visible:underline">{{
              t(item.name)
            }}</Link>
          </li>
        </ul>
      </section>
    </div>
    <p v-else class="rounded-lg border border-border-primary bg-card p-5 text-text-secondary">
      {{
        t(
          'Доступных разделов пока нет. Проверьте свои права доступа или откройте профиль через меню пользователя.',
        )
      }}
    </p>
  </main>
</template>

<script setup lang="ts">
import { formatDateTime, t } from '@/i18n'

import { computed } from 'vue'
import { Link, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import AppButton from '@/components/ui/AppButton.vue'
import DashboardCard from './DashboardCard.vue'
import { useDashboard, type DashboardSnapshot } from './useDashboard'
import { dashboardSections, type AdminMenu } from './dashboardSections'

defineProps<{ title?: string }>()
const page = usePage<{ adminMenu?: AdminMenu; dashboard?: DashboardSnapshot }>()
const { cards, updatedAt, refreshing, autoRefresh, failed, stopped, refresh } = useDashboard(
  page.props.dashboard,
)
const sections = computed(() => dashboardSections(page.props.adminMenu))
</script>
