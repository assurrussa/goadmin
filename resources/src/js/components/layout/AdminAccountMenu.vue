<script setup lang="ts">
import { t } from '@/i18n'
import { Link } from '@inertiajs/vue3'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { ChevronDown, Eye, Link2, LogOut, Settings, User } from 'lucide-vue-next'
import { useAdminHeader } from './useAdminHeader'
const {
  accountMenuItems,
  rolePreviewOpen,
  canPreviewRole,
  onMenuCloseAutoFocus,
  avatarUrl,
  primaryRoleLabel,
  userInitials,
  authuser,
  handleLogout,
} = useAdminHeader()
</script>
<template>
  <DropdownMenu>
    <DropdownMenuTrigger
      data-profile-menu-trigger
      class="menu-trigger flex items-center cursor-pointer gap-3 rounded-[var(--goadmin-menu-radius,1rem)] px-2.5 py-1.5 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-primary"
    >
      <div
        class="h-8 w-8 rounded-full bg-primary-dark dark:bg-primary flex items-center justify-center overflow-hidden"
      >
        <img
          v-if="avatarUrl"
          :src="avatarUrl"
          referrerpolicy="no-referrer"
          :alt="t('Аватар администратора')"
          class="h-full w-full object-cover"
        />
        <span v-else class="text-sm font-medium text-primary-contrast">
          {{ userInitials }}
        </span>
      </div>
      <div class="hidden sm:block text-left">
        <p class="text-sm font-medium text-text-primary">
          {{ authuser?.name ?? t('Пользователь') }}
        </p>
        <p class="text-xs text-text-secondary">
          {{ authuser?.email }}
        </p>
        <p v-if="primaryRoleLabel" class="text-[8px] text-text-tertiary uppercase tracking-wide">
          {{ primaryRoleLabel }}
        </p>
      </div>
      <ChevronDown class="h-4 w-4 text-text-tertiary" />
    </DropdownMenuTrigger>

    <DropdownMenuContent class="w-56" @close-auto-focus="onMenuCloseAutoFocus">
      <DropdownMenuItem as-child>
        <Link href="/auth/profile" class="flex items-center">
          <User class="mr-3 h-4 w-4" />
          {{ t('Профиль') }}
        </Link>
      </DropdownMenuItem>
      <DropdownMenuItem as-child>
        <Link href="/auth/profile/settings" class="flex items-center">
          <Settings class="mr-3 h-4 w-4" />
          {{ t('Настройки') }}
        </Link>
      </DropdownMenuItem>
      <DropdownMenuItem v-for="item in accountMenuItems" :key="item.href" as-child>
        <Link :href="item.href" class="flex items-center">
          <Link2 class="mr-3 h-4 w-4" />
          {{ t(item.name) }}
        </Link>
      </DropdownMenuItem>
      <DropdownMenuItem v-if="canPreviewRole" @select="rolePreviewOpen = true">
        <Eye class="mr-3 h-4 w-4" />
        {{ t('Предпросмотр роли') }}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem class="text-error focus:text-error" @click="handleLogout">
        <LogOut class="mr-3 h-4 w-4" />
        {{ t('Выйти') }}
      </DropdownMenuItem>
    </DropdownMenuContent>
  </DropdownMenu>
</template>
