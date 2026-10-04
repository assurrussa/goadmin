<template>
  <Teleport to="body">
    <div
      v-if="notifications.length > 0"
      class="fixed top-4 right-4 z-[9999] max-w-sm w-[calc(100%-2rem)] space-y-2"
    >
      <TransitionGroup name="notification" tag="div" class="space-y-2">
        <NotificationItem
          v-for="notification in notifications"
          :key="notification.id"
          :notification="notification"
          @close="removeNotification(notification.id)"
        />
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { storeToRefs } from 'pinia'
import { useNotificationsStore } from '@/stores/notifications'
import NotificationItem from './NotificationItem.vue'

const store = useNotificationsStore()
const { notifications } = storeToRefs(store)

const removeNotification = (id: string) => {
  store.remove(id)
}
</script>

<style scoped>
/* Анимации для уведомлений */
.notification-enter-active {
  transition: all 0.3s ease-out;
}

.notification-leave-active {
  transition: all 0.3s ease-in;
}

.notification-enter-from {
  transform: translateX(100%);
  opacity: 0;
}

.notification-leave-to {
  transform: translateX(100%);
  opacity: 0;
}

.notification-move {
  transition: transform 0.3s ease;
}
</style>
