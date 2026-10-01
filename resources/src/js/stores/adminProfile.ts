import { defineStore } from 'pinia'

export const useAdminProfileStore = defineStore('adminProfile', {
  state: () => ({
    avatarUrl: null as string | null,
  }),

  getters: {
    hasAvatar: (state) => !!state.avatarUrl,
  },

  actions: {
    setAvatarUrl(url: string | null | undefined) {
      this.avatarUrl = url || null
    },

    ensureAvatarUrl(initial?: string | null) {
      if (!this.avatarUrl && initial) {
        this.avatarUrl = initial
      }
    },
  },
})
