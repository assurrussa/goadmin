<template>
  <AppHead :title="props.title" />
  <PageActionBar back-href="/roles" label="Просмотр" />

  <div class="space-y-6">
    <div class="bg-card border border-border-primary shadow-sm sm:rounded-lg overflow-hidden">
      <div class="px-4 py-5 sm:p-6">
        <dl class="divide-y divide-border-secondary">
          <div class="py-4 grid grid-cols-3 gap-4">
            <dt class="text-sm font-medium text-text-secondary">ID</dt>
            <dd class="mt-0 text-sm text-text-primary col-span-2">{{ props.role.id }}</dd>
          </div>
          <div class="py-4 grid grid-cols-3 gap-4">
            <dt class="text-sm font-medium text-text-secondary">Slug</dt>
            <dd class="mt-0 text-sm text-text-primary col-span-2">{{ props.role.slug }}</dd>
          </div>
          <div class="py-4 grid grid-cols-3 gap-4">
            <dt class="text-sm font-medium text-text-secondary">Название</dt>
            <dd class="mt-0 text-sm text-text-primary col-span-2">{{ props.role.name }}</dd>
          </div>
          <div class="py-4 grid grid-cols-3 gap-4">
            <dt class="text-sm font-medium text-text-secondary">Описание</dt>
            <dd class="mt-0 text-sm text-text-primary col-span-2">
              {{ props.role.description || '—' }}
            </dd>
          </div>
          <div class="py-4 grid grid-cols-3 gap-4">
            <dt class="text-sm font-medium text-text-secondary">Тип</dt>
            <dd class="mt-0 text-sm text-text-primary col-span-2">
              <AppBadge :variant="props.role.isSystem ? 'warning' : 'success'">
                {{ props.role.isSystem ? 'Системная' : 'Пользовательская' }}
              </AppBadge>
            </dd>
          </div>
        </dl>
      </div>
    </div>

    <div class="bg-card border border-border-primary shadow-sm sm:rounded-2xl overflow-hidden">
      <div class="px-5 py-4 border-b border-border-secondary bg-surface-variant/40 space-y-1">
        <p class="text-xs uppercase tracking-wide text-text-tertiary mb-1">Работа с ролью</p>
        <h3 class="text-base font-semibold text-text-primary">Назначить роль администратору</h3>
        <p class="text-sm text-text-secondary">
          Админов немного, поэтому сразу доступны все, систему защищает ограничение на снятие роли у
          себя.
        </p>
      </div>
      <div class="px-5 py-5 space-y-5">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div class="flex-1 flex flex-col gap-2">
            <label class="text-xs font-medium uppercase tracking-wide text-text-tertiary">
              Поиск администратора
            </label>
            <AppInput v-model="searchQuery" type="text" placeholder="Имя, фамилия или email" />
          </div>
          <AppButton
            variant="secondary"
            size="sm"
            class="w-full sm:w-44 sm:self-end"
            type="button"
            :disabled="searchLoading"
            @click="searchAdmins"
          >
            {{ searchLoading ? 'Поиск…' : 'Фильтр' }}
          </AppButton>
        </div>
        <div class="space-y-3">
          <div
            v-if="adminOptions.length"
            class="border border-border-secondary rounded-2xl divide-y divide-border-secondary/60 overflow-hidden"
          >
            <div
              v-for="option in adminOptions"
              :key="option.id"
              class="flex flex-col gap-2 sm:flex-row sm:items-center sm:gap-4 px-4 py-3"
              :class="option.hasRole ? 'bg-surface-variant/30' : 'bg-surface'"
            >
              <div class="flex-1 min-w-0">
                <p class="text-sm font-semibold text-text-primary truncate">
                  {{ option.name }} {{ option.lastName }}
                </p>
                <div class="text-xs text-text-secondary flex flex-wrap gap-2">
                  <span>{{ option.email }}</span>
                  <span class="text-text-tertiary">@{{ option.username }}</span>
                </div>
              </div>
              <div
                v-if="option.roles && option.roles.length"
                class="flex flex-wrap gap-1 text-[11px] text-text-secondary max-w-full"
              >
                <span
                  v-for="role in option.roles"
                  :key="role"
                  class="px-2 py-0.5 rounded-full bg-surface-variant text-text-secondary"
                >
                  {{ role }}
                </span>
              </div>
              <div class="flex items-center gap-2 sm:w-44 sm:justify-end">
                <span v-if="option.hasRole" class="text-xs font-medium text-text-tertiary">
                  Уже назначено
                </span>
                <AppButton
                  variant="primary"
                  size="xs"
                  class="w-full sm:w-auto"
                  type="button"
                  :disabled="option.hasRole || attachingAdminId === option.id"
                  @click="handleAttach(option)"
                >
                  {{ attachingAdminId === option.id ? 'Назначаем…' : 'Назначить' }}
                </AppButton>
              </div>
            </div>
          </div>
          <p v-else class="text-sm text-text-secondary">
            Пока нечего показать — попробуйте уточнить поиск или обновите список.
          </p>
          <div class="space-y-1">
            <p class="text-xs text-text-tertiary">
              Выбирайте админа и добавляйте роль прямо из списка — можно несколько.
            </p>
            <p v-if="attachForm.errors.adminId" class="text-sm text-error">
              {{ attachForm.errors.adminId }}
            </p>
            <p v-else-if="attachError" class="text-sm text-error">{{ attachError }}</p>
          </div>
        </div>
      </div>
    </div>

    <div class="bg-card border border-border-primary shadow-sm sm:rounded-2xl overflow-hidden">
      <div
        class="px-5 py-4 border-b border-border-secondary flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between bg-surface-variant/40"
      >
        <div>
          <h3 class="text-base font-semibold text-text-primary">Администраторы с этой ролью</h3>
          <p class="text-sm text-text-secondary">
            {{
              assignedAdmins.length
                ? 'Список администраторов'
                : 'Роль пока не назначена ни одному администратору'
            }}
          </p>
        </div>
        <AppButton
          v-if="assignedAdmins.length"
          variant="secondary"
          size="sm"
          type="button"
          :disabled="refreshLoading"
          @click="refreshAdmins"
        >
          {{ refreshLoading ? 'Обновляем…' : 'Обновить список' }}
        </AppButton>
      </div>
      <div class="px-5 py-5">
        <template v-if="assignedAdmins.length">
          <div class="grid gap-4 sm:grid-cols-2">
            <article
              v-for="admin in assignedAdmins"
              :key="admin.id"
              class="border border-border-secondary rounded-2xl bg-surface-variant/30 px-4 py-3 flex flex-col gap-3"
            >
              <div class="flex items-start justify-between gap-3">
                <div>
                  <p class="text-sm font-semibold text-text-primary">
                    {{ admin.name }} {{ admin.lastName }}
                  </p>
                  <p class="text-sm text-text-secondary">{{ admin.email }}</p>
                </div>
                <span
                  class="text-xs font-medium text-text-tertiary bg-border-secondary/60 rounded-full px-2 py-0.5"
                >
                  @{{ admin.username }}
                </span>
              </div>
              <div class="flex items-center justify-between border-t border-border-secondary pt-3">
                <p class="text-xs text-text-tertiary">Права останутся, пока не удалите.</p>
                <AppButton
                  variant="danger"
                  size="sm"
                  type="button"
                  :disabled="pendingAdminId === admin.id"
                  @click="handleDetach(admin)"
                >
                  {{ pendingAdminId === admin.id ? 'Удаляем…' : 'Убрать роль' }}
                </AppButton>
              </div>
            </article>
          </div>
        </template>
        <p v-else class="text-sm text-text-secondary">Нет назначенных администраторов.</p>
      </div>
    </div>

    <RolePermissionsSection
      :permissions="props.role.permissions || []"
      :editable="false"
      variant="card"
      title="Права"
      subtitle="Список разрешений, связанных с ролью"
      header-eyebrow="Доступы роли"
      :show-total="true"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useForm } from '@inertiajs/vue3'
import AppHead from '@/components/layout/AppHead.vue'
import PageActionBar from '@/components/layout/PageActionBar.vue'
import RolePermissionsSection from '@/components/roles/RolePermissionsSection.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppInput from '@/components/ui/AppInput.vue'

interface PermissionDetail {
  domain: string
  action: string
  description?: string | null
}

interface RoleAssignedAdmin {
  id: number
  name: string
  lastName: string
  email: string
  username: string
}

interface AdminOption extends RoleAssignedAdmin {
  roles?: string[]
  hasRole: boolean
}

interface RoleDetail {
  id: number
  slug: string
  name: string
  description?: string | null
  isSystem: boolean
  permissions?: PermissionDetail[]
  admins?: RoleAssignedAdmin[]
}

const props = defineProps({
  title: {
    type: [String],
    default: undefined,
  },
  role: {
    type: Object as () => RoleDetail,
    required: true,
  },
})

const assignedAdmins = ref<RoleAssignedAdmin[]>([])
const adminOptions = ref<AdminOption[]>([])
const searchQuery = ref('')
const searchLoading = ref(false)
const assignedLoading = ref(false)
const attachError = ref('')
const pendingAdminId = ref<number | null>(null)
const attachingAdminId = ref<number | null>(null)
const refreshLoading = ref(false)
const attachForm = useForm<{ adminId: number | null }>({
  adminId: null,
})
const detachForm = useForm<{ adminId: number | null }>({
  adminId: null,
})
const syncAdminOptionsWithAssigned = () => {
  if (!adminOptions.value.length) {
    return
  }
  const assignedIds = new Set(assignedAdmins.value.map((admin) => admin.id))
  adminOptions.value = adminOptions.value.map((option) => ({
    ...option,
    hasRole: assignedIds.has(option.id),
  }))
}

watch(
  () => props.role?.admins,
  (admins) => {
    if (Array.isArray(admins)) {
      assignedAdmins.value = [...admins]
    } else {
      assignedAdmins.value = []
    }
    syncAdminOptionsWithAssigned()
  },
  { immediate: true },
)

const roleId = computed(() => props.role.id)

const loadAssignedAdmins = async () => {
  if (assignedLoading.value) {
    return
  }
  assignedLoading.value = true
  try {
    const resp = await fetch(`/roles/${roleId.value}/admins`)
    if (!resp.ok) {
      throw new Error('failed to fetch assigned admins')
    }
    const data = await resp.json()
    assignedAdmins.value = Array.isArray(data.admins) ? data.admins : []
    syncAdminOptionsWithAssigned()
  } catch (error) {
    console.error('Load assigned admins error', error)
    attachError.value = 'Не удалось загрузить назначенных администраторов'
  } finally {
    assignedLoading.value = false
  }
}

const refreshAdmins = () => {
  if (refreshLoading.value) {
    return
  }
  refreshLoading.value = true
  void loadAssignedAdmins().finally(() => {
    refreshLoading.value = false
  })
}

const searchAdmins = async () => {
  attachError.value = ''
  searchLoading.value = true
  try {
    const params = new URLSearchParams()
    if (searchQuery.value.trim()) {
      params.set('search', searchQuery.value.trim())
    }
    const resp = await fetch(`/roles/${roleId.value}/admins/search?${params.toString()}`)
    if (!resp.ok) {
      throw new Error('failed to fetch admins')
    }
    const data = await resp.json()
    adminOptions.value = Array.isArray(data.admins) ? data.admins : []
    syncAdminOptionsWithAssigned()
  } catch (error) {
    console.error('Search admins error', error)
    attachError.value = 'Не удалось загрузить администраторов'
  } finally {
    searchLoading.value = false
  }
}

const handleAttach = (admin: AdminOption) => {
  if (!admin?.id) {
    attachError.value = 'Не выбран администратор'
    return
  }
  if (admin.hasRole) {
    attachError.value = 'Эта роль уже назначена выбранному администратору'
    return
  }

  attachError.value = ''
  attachForm.clearErrors()
  attachingAdminId.value = admin.id
  attachForm.adminId = admin.id
  attachForm.post(`/roles/${roleId.value}/admins/attach`, {
    preserveScroll: true,
    preserveState: true,
    onError: (errors) => {
      attachError.value = (errors.adminId as string) || 'Не удалось назначить роль'
    },
    onSuccess: () => {
      void loadAssignedAdmins()
      void searchAdmins()
    },
    onFinish: () => {
      attachingAdminId.value = null
    },
  })
}

const handleDetach = (admin: RoleAssignedAdmin) => {
  pendingAdminId.value = admin.id
  attachError.value = ''
  detachForm.clearErrors()
  detachForm.adminId = admin.id
  detachForm.post(`/roles/${roleId.value}/admins/detach`, {
    preserveScroll: true,
    preserveState: true,
    onError: (errors) => {
      attachError.value = (errors.adminId as string) || 'Не удалось убрать роль у администратора'
    },
    onSuccess: () => {
      void loadAssignedAdmins()
      void searchAdmins()
    },
    onFinish: () => {
      pendingAdminId.value = null
    },
  })
}

onMounted(() => {
  void loadAssignedAdmins()
  void searchAdmins()
})
</script>
