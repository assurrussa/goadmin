<script setup lang="ts">
import { Link } from '@inertiajs/vue3'
import { t } from '@/i18n'
import {
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from '@/components/ui/dropdown-menu'
import { useAdminNavigation } from './useAdminNavigation'
type Item = { name: string; href?: string; children?: Item[] }
defineProps<{ items: Item[] }>()
const { isActive } = useAdminNavigation()
</script>
<template>
  <template v-for="(item, index) in items" :key="item.href || `${index}:${item.name}`">
    <DropdownMenuSub v-if="item.children?.length">
      <DropdownMenuSubTrigger>{{ t(item.name) }}</DropdownMenuSubTrigger>
      <DropdownMenuSubContent>
        <DropdownMenuItem v-if="item.href" as-child>
          <Link
            :href="item.href"
            :class="isActive(item.href) ? 'bg-primary/10 text-primary' : undefined"
            :aria-current="isActive(item.href) ? 'page' : undefined"
            >{{ t(item.name) }}</Link
          >
        </DropdownMenuItem>
        <AdminTopMenuItems :items="item.children" />
      </DropdownMenuSubContent>
    </DropdownMenuSub>
    <DropdownMenuItem v-else-if="item.href" as-child>
      <Link
        :href="item.href"
        :class="isActive(item.href) ? 'bg-primary/10 text-primary' : undefined"
        :aria-current="isActive(item.href) ? 'page' : undefined"
        >{{ t(item.name) }}</Link
      >
    </DropdownMenuItem>
    <div v-else class="px-2 py-1.5 text-xs text-text-tertiary">{{ t(item.name) }}</div>
  </template>
</template>
