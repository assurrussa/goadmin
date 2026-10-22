<template>
  <AppHead :title="pageTitle" />

  <div class="space-y-6">
    <div class="flex flex-wrap gap-2 items-center">
      <Link
        v-for="tab in tabs"
        :key="tab.href"
        :href="tab.href"
        :aria-current="isActive(tab.href) ? 'page' : undefined"
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

    <Card class="p-6 space-y-4">
      <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
        <div>
          <p class="text-xs font-semibold uppercase tracking-widest text-primary">
            {{ t('Очереди') }}
          </p>
          <h1 class="text-2xl font-bold text-text-primary">{{ pageTitle }}</h1>
          <p class="text-text-secondary mt-2">
            {{ t('Зарезервированные и ожидающие обработки задачи.') }}
          </p>
        </div>
        <div class="flex gap-3">
          <div class="px-4 py-3 rounded-lg bg-card border border-border-secondary">
            <p class="text-xs text-text-tertiary">{{ t('В очереди') }}</p>
            <p class="text-2xl font-semibold text-text-primary">{{ counters.jobs }}</p>
          </div>
          <div class="px-4 py-3 rounded-lg bg-error/5 border border-error/20">
            <p class="text-xs text-error">{{ t('С ошибкой') }}</p>
            <p class="text-2xl font-semibold text-error">{{ counters.failed }}</p>
          </div>
        </div>
      </div>

      <DataGrid
        ref="jobsGridRef"
        :api-url="jobsApi"
        :initial-data="null"
        @action-table="handleTableAction"
        @action-row="handleRowAction"
      />
    </Card>
  </div>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { computed, reactive, ref } from 'vue'
import { Link, router, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import DataGrid from '@/components/datagrid/DataGrid.vue'
import { Card } from '@/components/ui/card'

const props = defineProps({
  title: {
    type: [String],
    default: 'Активные задачи',
  },
  jobsApiUrl: {
    type: String,
    default: '/queues/jobs',
  },
  jobsCount: {
    type: Number,
    default: 0,
  },
  failedCount: {
    type: Number,
    default: 0,
  },
})

const jobsGridRef = ref<InstanceType<typeof DataGrid> | null>(null)
const page = usePage()

const tabs = [
  { label: 'Дашборд', href: '/queues' },
  { label: 'Активные задачи', href: '/queues/jobs' },
  { label: 'DLQ / Ошибки', href: '/queues/jobs-failed' },
]

const counters = reactive({
  jobs: Number(props.jobsCount ?? 0),
  failed: Number(props.failedCount ?? 0),
})

const pageTitle = computed(() => props.title || t('Активные задачи'))
const jobsApi = computed(() => props.jobsApiUrl || '/queues/jobs')
const isActive = (href: string) => {
  const pathname = page.url.split(/[?#]/)[0]
  if (href === '/queues') {
    return pathname === '/queues'
  }
  return pathname === href || pathname.startsWith(`${href}/`)
}
const clamp = (value: number) => (value < 0 ? 0 : value)

const extractId = (item: Record<string, unknown>): string | number | null => {
  const raw = item.id
  if (raw === null || raw === undefined) return null
  if (typeof raw === 'number' || typeof raw === 'string') return raw
  if (
    typeof raw === 'object' &&
    raw !== null &&
    typeof (raw as { toString?: () => string }).toString === 'function'
  ) {
    return (raw as { toString: () => string }).toString()
  }
  return null
}

const refreshJobs = () => jobsGridRef.value?.refreshData()

function handleTableAction(actionId: string) {
  if (actionId === 'refresh') {
    refreshJobs()
  }
}

function handleRowAction(actionKey: string, item: Record<string, unknown>) {
  const id = extractId(item)
  if (!id) return

  if (actionKey === 'delete') {
    if (!confirm(t('Удалить задачу #{id} из очереди?', undefined, { id: id }))) return

    router.delete(`${jobsApi.value}/${encodeURIComponent(String(id))}`, {
      preserveScroll: true,
      onSuccess: () => {
        refreshJobs()
        counters.jobs = clamp(counters.jobs - 1)
      },
    })
  }
}
</script>
