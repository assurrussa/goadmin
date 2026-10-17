<script setup lang="ts">
import { computed, ref } from 'vue'
import { usePage, router } from '@inertiajs/vue3'
import axios from 'axios'
import { useI18n, registerTranslations } from '@/i18n'
registerTranslations({
  ru: {},
  en: {
    'Предпросмотр роли': 'Role preview',
    'Только чтение. Ваша учётная запись и её роли не меняются.':
      'Read only. Your account and assigned roles are unchanged.',
    'Выйти из предпросмотра': 'Exit preview',
    'Выберите роль': 'Select a role',
    'Открыть предпросмотр': 'Open preview',
    'Не удалось изменить предпросмотр. Обновите страницу и проверьте состояние.':
      'Could not change preview. Reload and check its state.',
  },
})
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
  <aside
    v-if="preview?.available || preview?.active"
    class="border-b border-warning bg-surface-variant px-6 py-3 space-y-2"
    :aria-label="t('Предпросмотр роли')"
  >
    <template v-if="preview.active">
      <strong>{{ t('Предпросмотр роли') }}: {{ preview.roleName }}</strong>
      <p>{{ t('Только чтение. Ваша учётная запись и её роли не меняются.') }}</p>
      <button type="button" :disabled="busy" class="underline" @click="change(0)">
        {{ t('Выйти из предпросмотра') }}
      </button>
    </template>
    <form
      v-else
      class="flex flex-wrap items-center gap-3"
      @submit.prevent="change(Number(selected))"
    >
      <label
        >{{ t('Предпросмотр роли') }}
        <select v-model="selected" required class="rounded border bg-surface p-2">
          <option value="" disabled>{{ t('Выберите роль') }}</option>
          <option v-for="role in preview.roles" :key="role.id" :value="String(role.id)">
            {{ role.name }}
          </option>
        </select></label
      >
      <button type="submit" :disabled="busy || !selected" class="underline">
        {{ t('Открыть предпросмотр') }}
      </button>
    </form>
    <p v-if="error" role="alert">{{ error?.() }}</p>
  </aside>
</template>
