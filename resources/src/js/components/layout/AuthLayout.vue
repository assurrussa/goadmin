<template>
  <div class="min-h-screen bg-background flex flex-col justify-center">
    <div class="absolute top-4 right-4 z-50">
      <AppButton
        variant="ghost"
        size="icon-sm"
        :title="themeStore.isDark ? 'Переключить на светлую тему' : 'Переключить на темную тему'"
        @click="handleThemeToggle"
      >
        <Sun v-if="themeStore.isDark" class="h-5 w-5" />
        <Moon v-else class="h-5 w-5" />
      </AppButton>
    </div>

    <div class="w-full relative">
      <div class="flex pt-5 pb-2 justify-center w-full items-center">
        <span
          class="px-4 py-1.5 rounded-full border border-border-primary bg-card text-sm font-semibold text-text-primary"
        >
          Админка
        </span>
      </div>

      <h2 class="mt-6 text-center text-3xl font-bold tracking-tight text-text-primary">
        <slot name="title" />
      </h2>

      <p v-if="$slots.subtitle" class="mt-2 text-center text-sm text-text-secondary">
        <slot name="subtitle" />
      </p>
    </div>

    <div class="mt-8 sm:mx-auto sm:w-full sm:max-w-md">
      <div class="card py-8 px-4 sm:px-10">
        <slot />
      </div>

      <div v-if="$slots.footer" class="mt-6">
        <slot name="footer" />
      </div>
    </div>

    <NotificationsContainer />
    <UploadQueueContainer />
  </div>
</template>

<script setup lang="ts">
import AppButton from '@/components/ui/AppButton.vue'
import { Moon, Sun } from 'lucide-vue-next'
import { useThemeStore } from '@/stores/theme'
import NotificationsContainer from '@/components/NotificationsContainer.vue'
import UploadQueueContainer from '@/components/UploadQueueContainer.vue'
import { useFlashMessages } from '~/composables/useFlashMessages'

useFlashMessages()
const themeStore = useThemeStore()

const handleThemeToggle = () => {
  themeStore.toggle()
}
</script>
