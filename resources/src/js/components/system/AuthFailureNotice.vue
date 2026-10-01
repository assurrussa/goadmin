<script setup lang="ts">
defineProps<{ message: string }>()
defineEmits<{ dismiss: [] }>()
</script>

<template>
  <aside
    v-if="message"
    role="alert"
    aria-live="assertive"
    aria-atomic="true"
    class="fixed bottom-4 left-4 right-4 z-50 mx-auto max-w-xl rounded-lg border border-border-primary bg-card p-4 text-text-primary shadow-lg"
  >
    <p class="pr-6">{{ message }}</p>
    <button
      type="button"
      class="mt-3 rounded border border-border-primary px-3 py-1 focus-visible:outline"
      @click="$emit('dismiss')"
    >
      Закрыть уведомление
    </button>
  </aside>
</template>
