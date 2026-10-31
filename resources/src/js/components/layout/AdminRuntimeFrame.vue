<script setup lang="ts">
import { appearance } from '@/appearance/state'
import { t } from '@/i18n'
import { provideUploadTaskTracker } from '@/composables/useUploadTaskTracker'
import NotificationsContainer from '@/components/NotificationsContainer.vue'
import UploadQueueContainer from '@/components/UploadQueueContainer.vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { useFlashMessages } from '@/composables/useFlashMessages'

const { uploads } = useAdminCapabilities()
useFlashMessages()
provideUploadTaskTracker()
</script>

<template>
  <div
    v-if="
      appearance.owner && !appearance.readOnly && ['error', 'conflict'].includes(appearance.status)
    "
    role="alert"
    class="border-b border-border-primary bg-surface px-4 py-3 text-sm text-text-primary"
  >
    {{
      t(
        appearance.status === 'conflict'
          ? 'Настройки изменены в другом окне.'
          : 'Не удалось сохранить настройки интерфейса.',
      )
    }}
    <a href="/auth/profile/settings#interface" class="ml-2 underline underline-offset-2">{{
      t('Проверить настройки')
    }}</a>
  </div>
  <slot />
  <NotificationsContainer />
  <UploadQueueContainer v-if="uploads" />
</template>
