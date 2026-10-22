<script setup lang="ts">
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import { Menu } from 'lucide-vue-next'
defineProps<{ open: boolean }>()
defineEmits<{ toggle: [] }>()
</script>
<template>
  <AppButton
    variant="ghost"
    size="icon-sm"
    class="lg:hidden"
    :aria-label="t('Открыть меню')"
    :aria-expanded="open"
    aria-controls="admin-mobile-navigation"
    @click="$emit('toggle')"
    ><Menu class="size-5"
  /></AppButton>
</template>
