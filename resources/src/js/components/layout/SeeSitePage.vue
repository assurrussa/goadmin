<template>
  <a
    :href="props.href"
    target="_blank"
    class="text-primary hover:text-primary-dark my-5 inline-block"
  >
    {{ props.text }}
  </a>
</template>

<script setup lang="ts">
const props = defineProps({
  href: {
    type: String,
    required: true,
    default: '',
  },
  text: {
    type: String,
    required: false,
    default: 'Посмотреть на сайте',
  },
})
</script>
