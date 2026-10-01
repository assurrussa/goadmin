<template>
  <form
    @submit.prevent="handleSubmit"
    :class="formClasses"
    :aria-busy="loading ? 'true' : undefined"
    :data-loading="loading || undefined"
    v-bind="$attrs"
  >
    <fieldset :disabled="loading">
      <slot />
    </fieldset>
  </form>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  spacing?: 'sm' | 'md' | 'lg'
  loading?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  spacing: 'md',
  loading: false,
})

const emit = defineEmits<{
  submit: [event: Event]
}>()

const formClasses = computed(() => {
  const spacingClasses = {
    sm: 'space-y-4',
    md: 'space-y-6',
    lg: 'space-y-8',
  }

  return [spacingClasses[props.spacing], { 'pointer-events-none opacity-75': props.loading }]
})

const handleSubmit = (event: Event) => {
  if (!props.loading) {
    emit('submit', event)
  }
}
</script>
