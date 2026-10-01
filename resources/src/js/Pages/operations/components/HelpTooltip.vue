<template>
  <Tooltip>
    <TooltipTrigger as-child>
      <button
        type="button"
        class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full border border-border-secondary text-text-tertiary transition-colors hover:border-border-primary hover:bg-surface hover:text-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
        aria-label="Показать описание"
      >
        <CircleHelp class="h-3.5 w-3.5" />
      </button>
    </TooltipTrigger>
    <TooltipContent
      side="top"
      align="center"
      :side-offset="8"
      :collision-padding="16"
      class="w-[min(22rem,calc(100vw-2rem))] whitespace-normal break-words rounded-xl border border-border-primary bg-card px-3 py-2 text-left text-[12px] leading-5 text-text-primary shadow-xl"
    >
      {{ text }}
    </TooltipContent>
  </Tooltip>
</template>

<script setup lang="ts">
import { CircleHelp } from 'lucide-vue-next'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

defineProps<{
  text: string
}>()
</script>
