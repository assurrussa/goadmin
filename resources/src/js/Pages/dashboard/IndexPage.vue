<template>
  <AppHead :title="title || 'Главная'" />
  <main class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold text-text-primary">{{ title || 'Главная' }}</h1>
      <p class="mt-1 text-text-secondary">Доступные разделы управления</p>
    </div>
    <div v-if="sections.length" class="grid gap-4 md:grid-cols-2">
      <section
        v-for="section in sections"
        :key="section.key"
        class="rounded-lg border border-border-primary bg-card p-5"
      >
        <h2 class="mb-3 text-lg font-semibold text-text-primary">
          {{ section.title || 'Разделы' }}
        </h2>
        <ul class="space-y-2">
          <li v-for="item in section.links" :key="item.href">
            <Link :href="item.href" class="text-primary hover:underline focus-visible:underline">{{
              item.name
            }}</Link>
          </li>
        </ul>
      </section>
    </div>
    <p v-else class="rounded-lg border border-border-primary bg-card p-5 text-text-secondary">
      Доступных разделов пока нет. Проверьте свои права доступа или откройте профиль через меню
      пользователя.
    </p>
  </main>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Link, usePage } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import { dashboardSections, type AdminMenu } from './dashboardSections'

defineProps<{ title?: string }>()
const page = usePage<{ adminMenu?: AdminMenu }>()
const sections = computed(() => dashboardSections(page.props.adminMenu))
</script>
