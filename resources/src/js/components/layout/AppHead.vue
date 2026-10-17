<script setup lang="ts">
import { t } from '@/i18n'

import { Head } from '@inertiajs/vue3'

defineProps({ title: String })
</script>

<template>
  <Head :title="title ? `${t(title)} — ${t('Админка')}` : t('Админка')">
    <slot />
  </Head>
</template>
