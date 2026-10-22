import { defineAdminTheme } from '@admin-core/presentation'
import type { AdminExtension } from '@admin-core/extensions/types'
import AdminShell from '../AdminShell.vue'
import AuthShell from '../AuthShell.vue'
import '../theme.css'

export default {
  theme: defineAdminTheme({
    id: 'example.topbar',
    apiVersion: 1,
    adminShell: AdminShell,
    authShell: AuthShell,
  }),
} satisfies AdminExtension
