<template>
  <AppHead :title="pageTitle" />

  <TooltipProvider :delay-duration="120">
    <div class="space-y-6">
      <PageActionBar back-href="/" label="Операции" />

      <section
        class="rounded-xl border border-info/35 bg-info/10 px-5 py-4 text-sm"
        aria-label="Связь операций с CMS"
      >
        <h2 class="font-semibold text-text-primary">
          Операции совместимы с CMS, но решают задачу хоста
        </h2>
        <p class="mt-1 max-w-5xl leading-6 text-text-secondary">
          CMS управляет материалами, публикациями, меню, SEO и журналом изменений. Этот раздел
          обслуживает публичную доставку сайта: пересобирает маршруты и sitemap либо запрашивает
          redeploy фронтенда. Публикация CMS может пометить публичное состояние как требующее
          обновления, а «Операции» выполняют такое обновление вручную.
        </p>
      </section>

      <div
        class="grid items-start gap-5 xl:grid-cols-[minmax(0,0.94fr)_minmax(0,1.06fr)] 2xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]"
      >
        <Card class="border-border-primary bg-surface">
          <CardHeader class="gap-y-2 pb-4">
            <CardTitle class="text-base font-semibold text-text-primary md:text-lg">
              Ручные действия
            </CardTitle>
            <CardDescription class="max-w-2xl text-sm leading-6 text-text-secondary">
              Используйте эти операции, когда нужно принудительно обновить публичные артефакты или
              запросить redeploy фронтенда.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-3 pt-0">
            <div class="rounded-xl border border-border-secondary bg-card px-4 py-3.5">
              <div
                class="flex flex-col gap-3 lg:grid lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-4"
              >
                <div class="min-w-0 space-y-1">
                  <p class="text-sm font-medium text-text-primary">Routes Front</p>
                  <p class="text-sm leading-6 text-text-secondary">
                    Синхронно пересобирает `routes-front` и обновляет локальный кэш маршрутов.
                  </p>
                </div>
                <AppButton
                  type="button"
                  size="sm"
                  class="w-full sm:w-auto lg:self-center"
                  @click="runAction('/operations/routes/regenerate')"
                >
                  Regenerate routes
                </AppButton>
              </div>
            </div>

            <div class="rounded-xl border border-border-secondary bg-card px-4 py-3.5">
              <div
                class="flex flex-col gap-3 lg:grid lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-4"
              >
                <div class="min-w-0 space-y-1">
                  <p class="text-sm font-medium text-text-primary">Sitemap</p>
                  <p class="text-sm leading-6 text-text-secondary">
                    Пересобирает sitemap index и sitemap files в backend runtime.
                  </p>
                </div>
                <AppButton
                  type="button"
                  variant="secondary"
                  size="sm"
                  class="w-full sm:w-auto lg:self-center"
                  @click="runAction('/operations/sitemap/regenerate')"
                >
                  Regenerate sitemap
                </AppButton>
              </div>
            </div>

            <div class="rounded-xl border border-border-secondary bg-card px-4 py-3.5">
              <div
                class="flex flex-col gap-3 lg:grid lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start lg:gap-4"
              >
                <div class="min-w-0 space-y-1">
                  <p class="text-sm font-medium text-text-primary">Frontend Redeploy</p>
                  <p class="text-sm leading-6 text-text-secondary">
                    Отправляет ручной запрос на frontend rebuild/redeploy через настроенный trigger.
                  </p>
                  <p v-if="!canRedeploy" class="pt-0.5 text-xs leading-5 text-warning">
                    Trigger недоступен: проверьте `FRONTEND_REBUILD_ENABLE`, `DOKPLOY_*` или hook
                    config.
                  </p>
                </div>
                <AppButton
                  type="button"
                  variant="outline"
                  size="sm"
                  :disabled="!canRedeploy"
                  class="w-full sm:w-auto lg:self-center"
                  @click="runAction('/operations/frontend/redeploy')"
                >
                  Redeploy frontend
                </AppButton>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card class="border-border-primary bg-surface">
          <CardHeader class="gap-y-2 pb-4">
            <CardTitle class="text-base font-semibold text-text-primary md:text-lg">
              Frontend Build State
            </CardTitle>
            <CardDescription class="text-sm leading-6 text-text-secondary">
              Текущее состояние очереди пересборки публичного фронтенда.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-3 pt-0">
            <div class="rounded-xl border border-border-secondary bg-card px-4 py-3.5">
              <p class="text-[11px] uppercase tracking-[0.14em] text-text-tertiary">
                Итоговое состояние
              </p>
              <div
                class="mt-1.5 flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between"
              >
                <div class="min-w-0 flex items-center gap-2">
                  <p class="text-sm font-medium text-text-primary">{{ stateSummary.title }}</p>
                  <HelpTooltip :text="stateSummary.description" />
                </div>
                <div class="w-fit">
                  <AppBadge :variant="stateSummary.variant" class="w-fit whitespace-nowrap">
                    {{ stateSummary.badge }}
                  </AppBadge>
                </div>
              </div>
            </div>

            <div
              v-for="row in stateRows"
              :key="row.key"
              class="rounded-xl border border-border-secondary bg-card px-4 py-3.5"
            >
              <div
                class="flex flex-col gap-2.5 lg:flex-row lg:items-center lg:justify-between lg:gap-5"
              >
                <div class="min-w-0 space-y-1">
                  <p class="text-[11px] uppercase tracking-[0.12em] text-text-tertiary">
                    {{ row.eyebrow }}
                  </p>
                  <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <p class="text-sm font-medium text-text-primary">{{ row.label }}</p>
                    <HelpTooltip :text="row.description" />
                  </div>
                </div>
                <div class="min-w-0 flex flex-wrap items-center gap-3 md:justify-end">
                  <div v-if="row.badge" class="w-fit shrink-0">
                    <AppBadge :variant="row.badge.variant" class="w-fit whitespace-nowrap">
                      {{ row.badge.label }}
                    </AppBadge>
                  </div>
                  <p
                    :class="[row.valueClass, row.badge ? 'pl-1.5' : '']"
                    class="max-w-full break-words text-sm font-medium leading-5 text-text-primary lg:text-right"
                  >
                    {{ row.value }}
                  </p>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  </TooltipProvider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { router } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { TooltipProvider } from '@/components/ui/tooltip'
import HelpTooltip from './components/HelpTooltip.vue'

type FrontendState = {
  exists?: boolean
  scope?: string
  dirty?: boolean
  revision?: number
  lastMutationAt?: string
  lastRebuildRequestAt?: string
  buildLockUntil?: string
  updatedAt?: string
}

type FrontendRebuild = {
  enabled?: boolean
  hasTriggerConfig?: boolean
  canTrigger?: boolean
}

type BadgeVariant = 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'info' | 'outline'

type StateRow = {
  key: string
  eyebrow: string
  label: string
  description: string
  value: string
  valueClass?: string
  badge?: {
    label: string
    variant: BadgeVariant
  }
}

const props = defineProps<{
  title?: string
  frontendState: FrontendState
  frontendRebuild: FrontendRebuild
}>()

const pageTitle = computed(() => props.title ?? 'Операции')
const canRedeploy = computed(() => props.frontendRebuild?.canTrigger ?? false)

const triggerStatus = computed(() => {
  if (!props.frontendRebuild.enabled) {
    return {
      label: 'Выключен',
      variant: 'warning' as const,
      description:
        'Автоматический и ручной redeploy недоступен, пока `FRONTEND_REBUILD_ENABLE` выключен.',
    }
  }

  if (!props.frontendRebuild.hasTriggerConfig) {
    return {
      label: 'Неполная настройка',
      variant: 'warning' as const,
      description:
        'Rebuild включен, но trigger не сможет отработать без `DOKPLOY_*` или другого backend trigger config.',
    }
  }

  return {
    label: 'Готов к запуску',
    variant: 'success' as const,
    description:
      'Backend может отправлять ручной и автоматический запрос на rebuild/redeploy публичного фронтенда.',
  }
})

const stateSummary = computed(() => {
  if (!props.frontendState.exists) {
    return {
      title: 'Состояние очереди еще не инициализировано',
      description:
        'Запись в `frontend_build_states` пока не создана. Обычно она появляется после первой публичной мутации контента или первого rebuild trigger.',
      badge: 'Нет state row',
      variant: 'warning' as const,
    }
  }

  if (props.frontendState.dirty) {
    return {
      title: 'Публичный фронтенд помечен как требующий пересборки',
      description:
        'Есть новые изменения публичного контента. Следующий rebuild/redeploy должен забрать актуальные routes, sitemap и SSG output.',
      badge: 'Нужен rebuild',
      variant: 'warning' as const,
    }
  }

  return {
    title: 'Очередь пересборки сейчас чистая',
    description:
      'На текущий момент backend не видит новых публичных изменений, которые требовали бы повторного frontend build/redeploy.',
    badge: 'Синхронизировано',
    variant: 'success' as const,
  }
})

const stateRows = computed<StateRow[]>(() => [
  {
    key: 'stateRow',
    eyebrow: 'State Row',
    label: 'Запись состояния в БД',
    description: props.frontendState.exists
      ? `Строка состояния существует для scope \`${props.frontendState.scope || 'frontend'}\`. По ней backend понимает, нужен ли rebuild и действует ли lock.`
      : 'Строка состояния еще не создана. Это значит, что очередь пересборки пока не инициализировалась или еще ни разу не использовалась.',
    value: props.frontendState.exists ? 'Есть запись' : 'Запись отсутствует',
    badge: {
      label: props.frontendState.exists ? 'Present' : 'Missing',
      variant: props.frontendState.exists ? 'info' : 'warning',
    },
  },
  {
    key: 'dirty',
    eyebrow: 'Dirty Flag',
    label: 'Нужна ли пересборка',
    description: props.frontendState.dirty
      ? 'Флаг `dirty = true` означает, что после последней публичной контентной мутации фронтенд нужно заново пересобрать или redeploy-нуть.'
      : 'Флаг `dirty = false` означает, что новых публичных изменений после последней синхронизации сейчас нет.',
    value: props.frontendState.dirty ? 'Да, есть непримененные изменения' : 'Нет, очередь чистая',
    badge: {
      label: props.frontendState.dirty ? 'Dirty' : 'Clean',
      variant: props.frontendState.dirty ? 'warning' : 'success',
    },
  },
  {
    key: 'revision',
    eyebrow: 'Revision',
    label: 'Счетчик публичных изменений',
    description:
      'Ревизия растет при изменениях публичного контента. Она нужна для диагностики: видно, что state менялся, даже если rebuild еще не был запрошен.',
    value: String(props.frontendState.revision ?? 0),
    badge: {
      label: `rev ${props.frontendState.revision ?? 0}`,
      variant: 'secondary',
    },
  },
  {
    key: 'lastMutationAt',
    eyebrow: 'Last Mutation',
    label: 'Последняя контентная мутация',
    description: props.frontendState.lastMutationAt
      ? 'Когда backend в последний раз пометил публичный фронтенд как измененный из-за publish/update/unpublish или другой публичной мутации.'
      : 'Публичных мутаций, влияющих на rebuild state, пока не зафиксировано.',
    value: props.frontendState.lastMutationAt || '—',
    valueClass: 'font-mono text-[13px]',
  },
  {
    key: 'lastRebuildRequestAt',
    eyebrow: 'Last Rebuild Request',
    label: 'Последний запрос на rebuild',
    description: props.frontendState.lastRebuildRequestAt
      ? 'Когда backend в последний раз отправлял запрос на frontend rebuild/redeploy: автоматически или из раздела Operations.'
      : 'Запрос на rebuild/redeploy еще не отправлялся или пока не записан в state.',
    value: props.frontendState.lastRebuildRequestAt || '—',
    valueClass: 'font-mono text-[13px]',
  },
  {
    key: 'buildLockUntil',
    eyebrow: 'Build Lock',
    label: 'Блокировка повторного trigger',
    description: props.frontendState.buildLockUntil
      ? 'До этого времени повторный trigger будет отклоняться, чтобы не запускать несколько frontend redeploy подряд.'
      : 'Сейчас активного lock нет: повторный trigger не блокируется по времени.',
    value: props.frontendState.buildLockUntil || '—',
    valueClass: props.frontendState.buildLockUntil ? 'font-mono text-[13px]' : '',
    badge: props.frontendState.buildLockUntil
      ? {
          label: 'Lock active',
          variant: 'warning',
        }
      : {
          label: 'No lock',
          variant: 'success',
        },
  },
  {
    key: 'updatedAt',
    eyebrow: 'Updated At',
    label: 'Последнее обновление строки состояния',
    description: props.frontendState.updatedAt
      ? 'Когда backend последний раз переписывал саму строку состояния в БД: dirty flag, revision, lock или timestamp rebuild request.'
      : 'Строка состояния еще не обновлялась или не создана.',
    value: props.frontendState.updatedAt || '—',
    valueClass: 'font-mono text-[13px]',
  },
  {
    key: 'triggerStatus',
    eyebrow: 'Trigger Status',
    label: 'Готовность backend trigger',
    description: triggerStatus.value.description,
    value: triggerStatus.value.label,
    badge: {
      label: triggerStatus.value.label,
      variant: triggerStatus.value.variant,
    },
  },
])

function runAction(path: string) {
  router.post(path, {}, { preserveScroll: true })
}
</script>
