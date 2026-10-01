<template>
  <AppHead :title="pageTitle" />

  <div class="space-y-6">
    <div class="flex flex-wrap gap-2 items-center">
      <Link
        v-for="tab in tabs"
        :key="tab.href"
        :href="tab.href"
        class="px-3 py-2 rounded-lg text-sm font-medium border transition-colors"
        :class="
          isActive(tab.href)
            ? 'bg-primary text-primary-contrast border-primary'
            : 'bg-card text-text-secondary border-border-primary hover:text-text-primary'
        "
      >
        {{ tab.label }}
      </Link>
    </div>

    <Card class="bg-gradient-to-br from-primary/10 via-surface to-card border-border-primary p-6">
      <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <p class="text-xs font-semibold uppercase tracking-widest text-primary">
            Мониторинг очередей
          </p>
          <h1 class="text-2xl font-bold text-text-primary mt-1">{{ pageTitle }}</h1>
          <p class="text-text-secondary mt-2 max-w-2xl">
            Отслеживайте текущие задачи и быстро возвращайте в работу упавшие джобы.
          </p>
        </div>
        <div class="flex gap-3">
          <div class="px-4 py-3 rounded-lg bg-card border border-border-primary shadow-sm">
            <p class="text-xs text-text-tertiary">В очереди</p>
            <p class="text-2xl font-semibold text-text-primary">{{ counters.jobs }}</p>
            <Link class="text-sm text-primary hover:text-primary-dark" href="/queues/jobs"
              >Смотреть</Link
            >
          </div>
          <div class="px-4 py-3 rounded-lg bg-danger/10 border border-danger/30 shadow-sm">
            <p class="text-xs text-danger">С ошибкой</p>
            <p class="text-2xl font-semibold text-danger">{{ counters.failed }}</p>
            <Link class="text-sm text-danger hover:text-danger/80" href="/queues/jobs-failed"
              >Смотреть</Link
            >
          </div>
        </div>
      </div>
    </Card>

    <Card class="p-6 space-y-4">
      <h2 class="text-lg font-semibold text-text-primary">Быстрые действия</h2>
      <div class="flex flex-wrap gap-3">
        <AppButton asChild variant="primary">
          <Link href="/queues/jobs">Открыть активные задачи</Link>
        </AppButton>
        <AppButton asChild variant="secondary">
          <Link href="/queues/jobs-failed">Открыть DLQ / Ошибки</Link>
        </AppButton>
      </div>
      <p class="text-text-secondary text-sm">
        Используйте разделы выше для детального управления задачами очередей.
      </p>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive } from 'vue'
import { Link, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import { Card } from '@/components/ui/card'
import AppButton from '@/components/ui/AppButton.vue'

interface QueueStats {
  jobs?: number
  failed?: number
}

const props = defineProps({
  title: {
    type: [String],
    default: 'Очереди задач',
  },
  stats: {
    type: Object as () => QueueStats,
    default: () => ({ jobs: 0, failed: 0 }),
  },
})

const page = usePage()

const tabs = [
  { label: 'Дашборд', href: '/queues' },
  { label: 'Активные задачи', href: '/queues/jobs' },
  { label: 'DLQ / Ошибки', href: '/queues/jobs-failed' },
]

const counters = reactive({
  jobs: Number(props.stats?.jobs ?? 0),
  failed: Number(props.stats?.failed ?? 0),
})

const pageTitle = computed(() => props.title || 'Очереди задач')
const isActive = (href: string) => {
  if (href === '/queues') {
    return page.url === '/queues'
  }
  return page.url.startsWith(href)
}
</script>
