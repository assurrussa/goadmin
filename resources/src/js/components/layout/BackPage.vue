<template>
  <Link :href="props.href" class="inline-flex" v-bind="linkAttrs" @click="handleClick">
    <AppButton
      :variant="props.variant === 'action' ? 'ghost' : 'link'"
      size="sm"
      :class="attrs.class"
    >
      <slot>{{ props.text }}</slot>
    </AppButton>
  </Link>
</template>

<script setup lang="ts">
import { computed, useAttrs } from 'vue'
import { Link } from '@inertiajs/vue3'
import AppButton from '@/components/ui/AppButton.vue'

defineOptions({ inheritAttrs: false })

const attrs = useAttrs()

const props = defineProps({
  href: {
    type: String,
    required: true,
    default: '',
  },
  text: {
    type: String,
    required: false,
    default: 'Вернуться назад',
  },
  useHistory: {
    type: Boolean,
    required: false,
    default: true,
  },
  variant: {
    type: String,
    required: false,
    default: 'link',
    validator: (value: string) => ['link', 'action'].includes(value),
  },
})

const linkAttrs = computed(() => {
  const { class: _class, ...rest } = attrs as Record<string, unknown>
  void _class
  return rest
})

const handleClick = (event: MouseEvent) => {
  if (!props.useHistory) return
  if (typeof window === 'undefined') return
  if (event.defaultPrevented) return
  if (event.button !== 0) return
  if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
  if (window.history.length <= 1) return

  event.preventDefault()
  window.history.back()
}
</script>
