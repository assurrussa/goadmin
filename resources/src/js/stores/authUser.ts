import { defineStore } from 'pinia'
import type { AdminAuthUser } from '@/types/models'

export const useAuthUserStore = defineStore('authUser', {
  state: () => ({
    authUser: null as AdminAuthUser | null,
  }),

  getters: {
    isAuthenticated: (state) => state.authUser !== null,
    avatarUrl: (state) => state.authUser?.avatarUrl ?? '',
    id: (state) => state.authUser?.id ?? null,
  },

  actions: {
    setAuthUser(user: AdminAuthUser | null) {
      this.authUser = user ? { ...user } : null
    },

    setAvatarUrl(url: string | null | undefined) {
      if (!this.authUser) {
        return
      }

      this.authUser = {
        ...this.authUser,
        avatarUrl: url ?? '',
      }
    },
  },
})
