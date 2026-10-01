<template>
  <Teleport to="body">
    <div
      v-if="uploads.length > 0"
      class="fixed bottom-4 right-4 z-[9999] w-full max-w-sm space-y-2"
    >
      <TransitionGroup name="upload-toast" tag="div" class="space-y-2">
        <div
          v-for="upload in uploads"
          :key="upload.id"
          class="rounded-xl border border-border-primary bg-card bg-opacity-100 p-4 shadow-2xl"
        >
          <div class="flex items-start gap-3">
            <div class="status-badge" :class="statusBadgeClass(upload)">
              <span
                v-if="upload.status === 'uploading' || upload.status === 'processing'"
                class="status-spinner"
                aria-hidden="true"
              ></span>
              <svg
                v-else-if="upload.status === 'completed'"
                class="h-4 w-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="M5 13l4 4L19 7" />
              </svg>
              <svg
                v-else
                class="h-4 w-4"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
              >
                <path d="M12 9v4" />
                <path d="M12 17h.01" />
                <circle cx="12" cy="12" r="9" />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="text-sm font-semibold text-text-primary truncate">
                    {{ upload.label }}
                  </p>
                  <p class="text-xs text-text-tertiary">
                    {{ statusLabel(upload) }}
                  </p>
                </div>
                <AppButton
                  v-if="upload.status !== 'uploading'"
                  variant="ghost"
                  size="xs"
                  @click="remove(upload.id)"
                >
                  x
                </AppButton>
              </div>
              <div class="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-border-secondary/40">
                <div
                  class="h-full transition-[width] duration-200"
                  :class="progressClass(upload)"
                  :style="{ width: `${upload.progress}%` }"
                ></div>
              </div>
              <div class="mt-1 text-[11px] text-text-tertiary">{{ upload.progress }}%</div>
            </div>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import AppButton from '@/components/ui/AppButton.vue'
import { useUploadQueueStore, type UploadItem } from '@/stores/uploadQueue'

const store = useUploadQueueStore()
const { uploads } = storeToRefs(store)

const remove = (id: string) => {
  store.remove(id)
}

const statusBadgeClass = (upload: UploadItem) => {
  if (upload.status === 'completed') {
    return 'status-success'
  }
  if (upload.status === 'error') {
    return 'status-error'
  }
  return 'status-active'
}

const progressClass = (upload: UploadItem) => {
  if (upload.status === 'completed') {
    return 'bg-success'
  }
  if (upload.status === 'error') {
    return 'bg-error'
  }
  if (upload.status === 'processing') {
    return 'bg-warning'
  }
  return 'bg-primary'
}

const statusLabel = (upload: UploadItem) => {
  if (upload.status === 'completed') {
    return 'Upload complete'
  }
  if (upload.status === 'error') {
    return upload.error ? `Error: ${upload.error}` : 'Upload failed'
  }
  if (upload.status === 'processing') {
    return 'Processing file'
  }
  return 'Uploading...'
}
</script>

<style scoped>
.upload-toast-enter-active {
  transition: all 0.3s ease-out;
}

.upload-toast-leave-active {
  transition: all 0.3s ease-in;
}

.upload-toast-enter-from,
.upload-toast-leave-to {
  transform: translateY(12px);
  opacity: 0;
}

.upload-toast-move {
  transition: transform 0.3s ease;
}

.status-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 9999px;
  border: 1px solid rgba(148, 163, 184, 0.35);
  color: var(--color-primary);
  background: var(--color-surface-variant);
  flex-shrink: 0;
}

.status-badge.status-success {
  color: var(--color-success);
  background: rgba(34, 197, 94, 0.12);
  border-color: rgba(34, 197, 94, 0.35);
}

.status-badge.status-error {
  color: var(--color-error);
  background: rgba(239, 68, 68, 0.12);
  border-color: rgba(239, 68, 68, 0.35);
}

.status-spinner {
  width: 14px;
  height: 14px;
  border-radius: 9999px;
  border: 2px solid currentColor;
  border-top-color: transparent;
  animation: status-spin 0.9s linear infinite;
}

@keyframes status-spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
