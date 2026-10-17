<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Eye } from 'lucide-vue-next'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  DialogClose,
} from '@/components/ui/dialog'
import AppButton from '@/components/ui/AppButton.vue'
import { usePage, router } from '@inertiajs/vue3'
import axios from 'axios'
import { useI18n, registerTranslations } from '@/i18n'
registerTranslations({
  ru: {},
  en: {
    'Предпросмотр роли': 'Role preview',
    Отмена: 'Cancel',
    'Режим просмотра': 'Preview mode',
    'Только чтение. Ваша учётная запись и её роли не меняются.':
      'Read only. Your account and assigned roles are unchanged.',
    'Выйти из предпросмотра': 'Exit preview',
    'Выберите роль': 'Select a role',
    'Открыть предпросмотр': 'Open preview',
    'Не удалось изменить предпросмотр. Обновите страницу и проверьте состояние.':
      'Could not change preview. Reload and check its state.',
  },
})
const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()
const page = usePage()
const preview = computed(
  () =>
    page.props.rolePreview as
      | {
          active: boolean
          available: boolean
          roleName?: string
          roles: { id: number; name: string }[]
        }
      | undefined,
)
const selected = ref('')
const busy = ref(false)
const error = ref<(() => string) | null>(null)
watch(open, (value) => {
  if (!value && !busy.value) {
    selected.value = ''
    error.value = null
  }
})
watch(
  () => page.url,
  () => {
    open.value = false
  },
)
function restoreFocus(event: Event) {
  event.preventDefault()
  document.querySelector<HTMLElement>('[data-profile-menu-trigger]')?.focus()
}
async function change(roleId: number) {
  if (busy.value) return
  busy.value = true
  error.value = null
  try {
    await axios.post('/auth/role-preview', { roleId })
    router.visit('/auth/profile', { replace: true })
  } catch {
    error.value = () =>
      t('Не удалось изменить предпросмотр. Обновите страницу и проверьте состояние.')
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <template v-if="preview?.available || preview?.active">
    <button
      v-if="preview.active"
      type="button"
      class="inline-flex max-w-48 items-center gap-2 rounded-lg border border-warning bg-surface-variant px-2.5 py-1.5 text-xs font-medium"
      :title="t('Только чтение. Ваша учётная запись и её роли не меняются.')"
      @click="open = true"
    >
      <Eye class="h-4 w-4 shrink-0" aria-hidden="true" />
      <span class="truncate">{{ t('Режим просмотра') }}: {{ preview.roleName }}</span>
    </button>
    <Dialog v-model:open="open">
      <DialogContent
        class="max-w-[calc(100%_-_2rem)] sm:max-w-lg rounded-xl border-border-primary"
        @close-auto-focus="restoreFocus"
      >
        <DialogHeader>
          <DialogTitle>{{ t('Предпросмотр роли') }}</DialogTitle>
          <DialogDescription>{{
            t('Только чтение. Ваша учётная запись и её роли не меняются.')
          }}</DialogDescription>
        </DialogHeader>
        <template v-if="preview.active">
          <p class="text-sm font-medium">{{ preview.roleName }}</p>
          <DialogFooter class="gap-3">
            <AppButton type="button" :disabled="busy" @click="change(0)">{{
              t('Выйти из предпросмотра')
            }}</AppButton>
          </DialogFooter>
        </template>
        <form v-else class="space-y-5" @submit.prevent="change(Number(selected))">
          <label class="block space-y-2 text-sm font-medium">
            <span>{{ t('Выберите роль') }}</span>
            <select
              v-model="selected"
              required
              :disabled="busy"
              class="w-full rounded-lg border border-border-primary bg-card px-3 py-2.5 text-text-primary focus:ring-2 focus:ring-primary"
            >
              <option value="" disabled>{{ t('Выберите роль') }}</option>
              <option v-for="role in preview.roles" :key="role.id" :value="String(role.id)">
                {{ role.name }}
              </option>
            </select>
          </label>
          <DialogFooter class="gap-3">
            <DialogClose as-child
              ><AppButton type="button" variant="secondary" :disabled="busy">{{
                t('Отмена')
              }}</AppButton></DialogClose
            >
            <AppButton type="submit" :disabled="busy || !selected">{{
              t('Открыть предпросмотр')
            }}</AppButton>
          </DialogFooter>
        </form>
        <p v-if="error" role="alert" class="text-sm text-error">{{ error?.() }}</p>
      </DialogContent>
    </Dialog>
  </template>
</template>
