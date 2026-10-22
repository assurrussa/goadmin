<script setup lang="ts">
import { t } from '@/i18n'

import type { DialogContentEmits, DialogContentProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { X } from 'lucide-vue-next'
import {
  DialogClose,
  DialogContent,
  DialogOverlay,
  DialogPortal,
  useForwardPropsEmits,
} from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<DialogContentProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<DialogContentEmits>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardPropsEmits(delegatedProps, emits)
</script>

<template>
  <DialogPortal>
    <DialogOverlay
      data-ui="dialog"
      data-part="overlay"
      class="fixed inset-0 z-50 grid place-items-center overflow-y-auto bg-[var(--goadmin-dialog-overlay,var(--color-overlay))] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0"
    >
      <DialogContent
        data-ui="dialog"
        data-part="content"
        :class="
          cn(
            'relative z-50 grid w-[calc(100%-2rem)] max-w-lg my-8 gap-4 rounded-[var(--goadmin-dialog-radius,var(--radius-xl))] border border-[var(--goadmin-dialog-border,var(--color-border))] bg-[var(--goadmin-dialog-bg,var(--color-card))] text-[var(--goadmin-dialog-text,var(--color-card-foreground))] p-[var(--goadmin-dialog-padding,1.5rem)] shadow-[var(--goadmin-dialog-shadow,0_10px_15px_-3px_rgb(0_0_0/0.1),0_4px_6px_-4px_rgb(0_0_0/0.1))] duration-200',
            props.class,
          )
        "
        v-bind="forwarded"
        @pointer-down-outside="
          (event) => {
            const originalEvent = event.detail.originalEvent
            const target = originalEvent.target as HTMLElement
            if (
              originalEvent.offsetX > target.clientWidth ||
              originalEvent.offsetY > target.clientHeight
            ) {
              event.preventDefault()
            }
          }
        "
      >
        <slot />

        <DialogClose
          data-ui="dialog"
          data-part="close"
          class="absolute top-4 right-4 p-0.5 transition-colors rounded-md hover:bg-secondary"
        >
          <X class="w-4 h-4" />
          <span class="sr-only">{{ t('core.ui.Close', 'Close') }}</span>
        </DialogClose>
      </DialogContent>
    </DialogOverlay>
  </DialogPortal>
</template>
