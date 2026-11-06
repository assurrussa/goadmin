<script setup lang="ts">
import { computed } from 'vue'
import { AlertTriangle, CircleHelp } from 'lucide-vue-next'
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { ConfirmationController } from './confirmation'

const props = defineProps<{ controller: ConfirmationController }>()
// Each rendered dialog captures its request, so a late close/double-click from
// an old dialog can never settle a newer confirmation.
const requests = computed(() =>
  props.controller.active.value ? [props.controller.active.value] : [],
)
</script>

<template>
  <Dialog
    v-for="request in requests"
    :key="request.id"
    :open="true"
    @update:open="
      (open) => {
        if (!open) controller.settle(request.id, false)
      }
    "
  >
    <DialogContent
      aria-modal="true"
      class="goadmin-confirmation-dialog max-h-[calc(100dvh-2rem)] max-w-md overflow-y-auto"
    >
      <DialogHeader>
        <div
          class="mb-2 flex h-10 w-10 items-center justify-center rounded-lg"
          :class="
            request.variant === 'danger' ? 'bg-error/10 text-error' : 'bg-primary/10 text-primary'
          "
          aria-hidden="true"
        >
          <AlertTriangle v-if="request.variant === 'danger'" class="h-5 w-5" />
          <CircleHelp v-else class="h-5 w-5" />
        </div>
        <DialogTitle>{{ t(request.title || 'Подтвердите действие') }}</DialogTitle>
        <DialogDescription class="break-words">{{
          t(request.message, undefined, request.params)
        }}</DialogDescription>
      </DialogHeader>
      <DialogFooter class="gap-2">
        <AppButton
          variant="secondary"
          data-confirmation-cancel
          @click="controller.settle(request.id, false)"
          >{{ t('Отмена') }}</AppButton
        >
        <AppButton
          :variant="request.variant === 'danger' ? 'danger' : 'primary'"
          data-confirmation-accept
          @click="controller.settle(request.id, true)"
          >{{ t(request.confirmLabel || 'Подтвердить') }}</AppButton
        >
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
