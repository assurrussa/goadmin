<script setup lang="ts">
import NotificationsContainer from '@/components/NotificationsContainer.vue'
import UploadQueueContainer from '@/components/UploadQueueContainer.vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { useFlashMessages } from '@/composables/useFlashMessages'

const { uploads } = useAdminCapabilities()
useFlashMessages()
</script>

<template>
  <slot />
  <NotificationsContainer />
  <UploadQueueContainer v-if="uploads" />
</template>
