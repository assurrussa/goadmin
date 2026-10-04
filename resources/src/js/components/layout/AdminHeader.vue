<template>
  <header class="bg-card border-b border-border-primary shadow-sm">
    <div
      class="flex items-center justify-between px-4 py-1 sm:px-6 lg:px-8 max-w-7xl mx-auto w-full min-h-[68px]"
    >
      <div class="flex items-center">
        <AppButton
          variant="ghost"
          size="icon-sm"
          class="lg:hidden"
          aria-label="Открыть меню"
          @click="$emit('toggle-sidebar')"
        >
          <Menu class="h-5 w-5" />
        </AppButton>
      </div>

      <div class="flex items-center gap-2 sm:gap-3">
        <AppButton
          variant="ghost"
          size="icon-sm"
          :aria-label="
            themeStore.isDark ? 'Переключить на светлую тему' : 'Переключить на темную тему'
          "
          :title="themeStore.isDark ? 'Переключить на светлую тему' : 'Переключить на темную тему'"
          @click="handleThemeToggle"
        >
          <Sun v-if="themeStore.isDark" class="h-5 w-5" />
          <Moon v-else class="h-5 w-5" />
        </AppButton>

        <Link v-if="notifications" href="/notifications" class="relative">
          <AppButton variant="ghost" size="icon-sm" aria-label="Открыть уведомления">
            <Bell class="h-5 w-5" />
          </AppButton>
          <Badge
            v-if="unreadNotifications > 0"
            variant="destructive"
            class="absolute -top-1 -right-1 min-w-[16px] h-4 px-1 rounded-full text-[10px] flex items-center justify-center p-0"
          >
            {{ unreadNotifications }}
          </Badge>
        </Link>

        <DropdownMenu>
          <DropdownMenuTrigger
            class="menu-trigger flex items-center cursor-pointer gap-3 rounded-2xl px-2.5 py-1.5 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-primary"
          >
            <div
              class="h-8 w-8 rounded-full bg-primary-dark dark:bg-primary flex items-center justify-center overflow-hidden"
            >
              <img
                v-if="avatarUrl"
                :src="avatarUrl"
                alt="Аватар администратора"
                class="h-full w-full object-cover"
              />
              <span v-else class="text-sm font-medium text-primary-contrast">
                {{ userInitials }}
              </span>
            </div>
            <div class="hidden sm:block text-left">
              <p class="text-sm font-medium text-text-primary">
                {{ authuser?.name ?? 'Пользователь' }}
              </p>
              <p class="text-xs text-text-secondary">
                {{ authuser?.email }}
              </p>
              <p
                v-if="primaryRoleLabel"
                class="text-[8px] text-text-tertiary uppercase tracking-wide"
              >
                {{ primaryRoleLabel }}
              </p>
            </div>
            <ChevronDown class="h-4 w-4 text-text-tertiary" />
          </DropdownMenuTrigger>

          <DropdownMenuContent class="w-56">
            <DropdownMenuItem as-child>
              <Link href="/auth/profile" class="flex items-center">
                <User class="mr-3 h-4 w-4" />
                Профиль
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem as-child>
              <Link href="/auth/profile/settings" class="flex items-center">
                <Settings class="mr-3 h-4 w-4" />
                Настройки
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem class="text-error focus:text-error" @click="handleLogout">
              <LogOut class="mr-3 h-4 w-4" />
              Выйти
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAdminCapabilities } from '@/composables/useAdminCapabilities'
import { Link, router, usePage } from '@inertiajs/vue3'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Badge } from '@/components/ui/badge'
import AppButton from '@/components/ui/AppButton.vue'
import { Bell, ChevronDown, LogOut, Menu, Moon, Settings, Sun, User } from 'lucide-vue-next'
import { useThemeStore } from '@/stores/theme'
import { useAuthUserStore } from '@/stores/authUser'
import type { AdminAuthUser } from '@/types/models.ts'

const { notifications } = useAdminCapabilities()
const authUserStore = useAuthUserStore()
const authuser = computed<AdminAuthUser | null>(() => authUserStore.authUser)
const page = usePage<{ notificationsSummary?: { unread?: number } }>()

defineEmits<{
  'toggle-sidebar': []
}>()
const themeStore = useThemeStore()

// Computed
const avatarUrl = computed(() => authUserStore.avatarUrl)
const formatRoleLabel = (role?: string): string => {
  if (!role) return ''
  return role
    .split('_')
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}
const primaryRoleLabel = computed(() => {
  const roles = authuser.value?.roles
  if (!roles || roles.length === 0) {
    return ''
  }
  return formatRoleLabel(roles[0])
})

const userInitials = computed(() => {
  const name = authuser.value?.name ?? 'U'
  return name
    .split(' ')
    .map((part) => part.charAt(0))
    .join('')
    .toUpperCase()
    .slice(0, 2)
})

const unreadNotifications = computed(() => {
  const summary = page.props.notificationsSummary
  if (!summary) return 0
  const value = (summary as { unread?: number }).unread
  return typeof value === 'number' ? value : 0
})

// Methods
const handleLogout = async () => {
  router.delete('/auth/logout')
}

const handleThemeToggle = () => {
  themeStore.toggle()
}
</script>
