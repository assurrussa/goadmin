<template>
  <nav
    v-if="visibleCrumbs.length > 1"
    class="flex items-center gap-2 text-xs text-text-tertiary"
    :aria-label="t('Хлебные крошки')"
  >
    <template v-for="(crumb, index) in visibleCrumbs" :key="crumbKey(crumb, index)">
      <Link
        v-if="crumb.href && index < visibleCrumbs.length - 1"
        :href="crumb.href"
        class="transition-colors hover:text-text-primary"
      >
        {{ t(crumb.name) }}
      </Link>
      <span v-else class="text-text-secondary font-medium">
        {{ t(crumb.name) }}
      </span>
      <ChevronRightIcon
        v-if="index < visibleCrumbs.length - 1"
        class="h-3.5 w-3.5 text-border-primary"
        aria-hidden="true"
      />
    </template>
  </nav>
</template>

<script setup lang="ts">
import { t } from '@/i18n'

import { computed } from 'vue'
import { Link, usePage } from '@inertiajs/vue3'
import { ChevronRightIcon } from '@heroicons/vue/24/outline'

type Breadcrumb = {
  name: string
  href?: string
}

const page = usePage<{ adminBreadcrumbs?: Breadcrumb[] }>()

const visibleCrumbs = computed(() => page.props.adminBreadcrumbs ?? [])

const crumbKey = (crumb: Breadcrumb, index: number): string => {
  if (crumb.href) return crumb.href
  return `${index}:${crumb.name}`
}
</script>
