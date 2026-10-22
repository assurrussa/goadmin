<script setup lang="ts">
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
  <slot />
  <NotificationsContainer />
  <UploadQueueContainer v-if="uploads" />
</template>
