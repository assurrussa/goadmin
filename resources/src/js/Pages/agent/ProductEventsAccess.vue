<script setup lang="ts">
import ReadAccess from './ReadAccess.vue'
defineProps<{ grants: string[] }>()
</script>

<template>
  <ReadAccess
    :grants="grants"
    connect-path="/agent/product-events/connect"
    title="Product journal connections"
    description="Enter the code from your local MCP client to allow it to read your product events and their unpublish reasons."
    back-href="/product-events"
    back-text="Журнал действий с товарами"
  />
</template>
