<template>
  <a
    :href="href"
    class="mb-4 block rounded-lg border border-primary px-4 py-3 text-center text-primary"
    >{{ t('Войти через SSO') }}</a
  >
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { usePage } from '@inertiajs/vue3'
import { t } from '@/i18n'
const props = defineProps<{ returnTo?: string }>()
const page = usePage()
const href = computed(() => {
  const query = page.url.includes('?') ? page.url.slice(page.url.indexOf('?') + 1) : ''
  const target = props.returnTo || new URLSearchParams(query).get('redirectback') || '/'
  return '/sso?redirectback=' + encodeURIComponent(target)
})
</script>
