<template>
  <div
    class="mb-6 mt-2 flex flex-wrap items-center gap-2 rounded-2xl border border-border-primary/80 bg-card px-3 py-2 shadow-sm sm:px-4 sm:py-3"
  >
    <BackPage :href="backHref" variant="action" :use-history="useHistory">
      <ArrowLeft class="h-4 w-4" />
      {{ backText }}
    </BackPage>
    <a v-if="siteHref" :href="siteHref" target="_blank" rel="noreferrer" class="inline-flex">
      <AppButton variant="secondary" size="sm" class="gap-2">
        <ArrowUpRight class="h-4 w-4" />
        {{ siteText }}
      </AppButton>
    </a>
    <div v-if="siteHref" class="hidden h-5 w-px bg-border-primary/70 sm:block" aria-hidden="true" />
    <span
      v-if="label"
      class="ml-auto text-xs font-medium uppercase tracking-wide text-text-tertiary"
    >
      {{ label }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { ArrowLeft, ArrowUpRight } from 'lucide-vue-next'
import BackPage from '@/components/layout/BackPage.vue'
import AppButton from '@/components/ui/AppButton.vue'

const props = defineProps({
  backHref: {
    type: String,
    required: true,
  },
  backText: {
    type: String,
    required: false,
    default: 'Вернуться назад',
  },
  siteHref: {
    type: String,
    required: false,
    default: '',
  },
  siteText: {
    type: String,
    required: false,
    default: 'Посмотреть на сайте',
  },
  label: {
    type: String,
    required: false,
    default: '',
  },
  useHistory: {
    type: Boolean,
    required: false,
    default: true,
  },
})

const backHref = props.backHref
const backText = props.backText
const siteHref = props.siteHref
const siteText = props.siteText
const label = props.label
const useHistory = props.useHistory
</script>
