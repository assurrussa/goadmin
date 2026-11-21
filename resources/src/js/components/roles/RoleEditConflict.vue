<script setup lang="ts">
import { computed } from 'vue'
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import { permissionLabel } from './permissionLabels'

interface RoleFields {
  name: string
  slug: string
  description: string
  isSystem: boolean
  permissions: string[]
}

const props = defineProps<{ saved: RoleFields; draft: RoleFields }>()
defineEmits<{ reapply: []; discard: [] }>()

const fields = [
  { key: 'name', label: 'Название' },
  { key: 'slug', label: 'Slug' },
  { key: 'description', label: 'Описание' },
  { key: 'isSystem', label: 'Тип' },
] as const
const changed = computed(() => fields.filter(({ key }) => props.saved[key] !== props.draft[key]))
const savedOnly = computed(() =>
  props.saved.permissions.filter((key) => !props.draft.permissions.includes(key)),
)
const draftOnly = computed(() =>
  props.draft.permissions.filter((key) => !props.saved.permissions.includes(key)),
)

function describe(value: string | boolean) {
  if (typeof value === 'boolean') return value ? t('Системная') : t('Пользовательская')
  return value || '—'
}
</script>

<template>
  <section
    aria-labelledby="role-conflict-heading"
    class="space-y-4 rounded-lg border border-warning/40 bg-warning/5 p-4"
  >
    <h2 id="role-conflict-heading" class="font-semibold text-text-primary" role="alert">
      {{ t('Роль изменилась после открытия формы') }}
    </h2>
    <p class="text-sm text-text-secondary">
      {{
        t(
          'Сравните сохранённую роль с черновиком. После выбора черновика проверьте его и сохраните снова.',
        )
      }}
    </p>
    <div class="grid gap-4 sm:grid-cols-2">
      <div v-for="side in ['saved', 'draft'] as const" :key="side" class="min-w-0 space-y-2">
        <h3 class="text-sm font-semibold text-text-primary">
          {{ side === 'saved' ? t('Сохранено') : t('Ваш черновик') }}
        </h3>
        <dl v-if="changed.length" class="space-y-2 text-sm">
          <div v-for="field in changed" :key="field.key">
            <dt class="text-text-secondary">{{ t(field.label) }}</dt>
            <dd class="whitespace-pre-wrap break-words text-text-primary">
              {{ describe(props[side][field.key]) }}
            </dd>
          </div>
        </dl>
        <h4 class="text-sm font-medium text-text-primary">
          {{
            side === 'saved' ? t('Права только в сохранённой роли') : t('Права только в черновике')
          }}
        </h4>
        <ul class="space-y-1 text-sm text-text-secondary">
          <li v-for="key in side === 'saved' ? savedOnly : draftOnly" :key="key" class="min-w-0">
            {{ permissionLabel(key) }}
            <code class="block break-all text-xs">{{ key }}</code>
          </li>
          <li v-if="!(side === 'saved' ? savedOnly : draftOnly).length">—</li>
        </ul>
      </div>
    </div>
    <div class="flex flex-wrap gap-3">
      <AppButton type="button" variant="secondary" @click="$emit('discard')">
        {{ t('Принять сохранённую версию') }}
      </AppButton>
      <AppButton type="button" variant="outline" @click="$emit('reapply')">
        {{ t('Оставить мой черновик') }}
      </AppButton>
    </div>
  </section>
</template>
