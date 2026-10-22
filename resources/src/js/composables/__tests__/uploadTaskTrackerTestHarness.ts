import { createApp, defineComponent, h } from 'vue'
import { useAuthUserStore } from '@/stores/authUser'
import { provideUploadTaskTracker } from '../useUploadTaskTracker'

export const uploadTestOwner = {
  id: 'upload-test-owner',
  name: '',
  lastName: '',
  email: '',
  avatarUrl: '',
  roles: [],
}

/** Standalone widget tests explicitly install the same lifetime owner as the runtime frame. */
export const createUploadTestApp: typeof createApp = (component, props) =>
  createApp(
    defineComponent({
      setup() {
        const auth = useAuthUserStore()
        if (!auth.authUser) auth.setAuthUser(uploadTestOwner)
        provideUploadTaskTracker()
        return () => h(component, props)
      },
    }),
  )
