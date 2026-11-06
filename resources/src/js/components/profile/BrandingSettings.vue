<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { adminFetch } from '@/http'
import { appearance, paintAppearance } from '@/appearance/state'
import {
  clearBrandingRecovery,
  hasBrandingRecovery,
  registerBrandingRecovery,
  takeBrandingRecovery,
} from '@/appearance/brandingRecovery'
import { deriveBrandPalette, readBrand, themePrimaryHex } from '@/appearance/brand'
import { AppearanceSaveError, notifySaveFailure } from '@/appearance/saveFeedback'
import { t } from '@/i18n'
import AppButton from '@/components/ui/AppButton.vue'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
const draft = reactive({
  name: '',
  accentColor: '',
  accentColorEnd: '',
  logoLight: undefined as string | undefined,
  logoDark: undefined as string | undefined,
})
const urls = reactive({ light: '', dark: '' })
const decoding = reactive({ light: false, dark: false })
const logoSequence = { light: 0, dark: 0 }
const decodingLogo = computed(() => decoding.light || decoding.dark)
function invalidateLogos() {
  logoSequence.light++
  logoSequence.dark++
  decoding.light = false
  decoding.dark = false
}
const version = ref(0)
const loaded = ref(false)
const resetOpen = ref(false)
const pickerColor = computed(() => {
  if (/^#[\da-f]{6}$/i.test(draft.accentColor)) return draft.accentColor
  // Read the host pack, not the currently applied brand override.
  return themePrimaryHex(appearance.dark) || '#2563eb'
})
function setPickerColor(event: Event) {
  draft.accentColor = (event.target as HTMLInputElement).value
}
function resetColors() {
  draft.accentColor = ''
  draft.accentColorEnd = ''
}
function setAccentMode(event: Event) {
  draft.accentColorEnd =
    (event.target as HTMLSelectElement).value === 'gradient'
      ? draft.accentColor || pickerColor.value || '#2563eb'
      : ''
  if (draft.accentColorEnd && !draft.accentColor) draft.accentColor = draft.accentColorEnd
}
function setEndPickerColor(event: Event) {
  draft.accentColorEnd = (event.target as HTMLInputElement).value
  if (!draft.accentColor) draft.accentColor = pickerColor.value || draft.accentColorEnd
}

let baseline = { name: '', accentColor: '', accentColorEnd: '' }
const status = ref('loading')
const error = ref('')
const pending = ref(false)
let alive = true
let epoch = 0
let controller: AbortController | undefined
const permission = computed(() => appearance.brandingUpdate && !appearance.readOnly)
const session = () =>
  `${appearance.owner}:${appearance.readOnly}:${appearance.brandingRead}:${appearance.brandingUpdate}`
watch(
  session,
  () => {
    const token = ++epoch
    invalidateLogos()
    controller?.abort()
    Object.assign(draft, {
      name: '',
      accentColor: '',
      accentColorEnd: '',
      logoLight: undefined,
      logoDark: undefined,
    })
    urls.light = ''
    urls.dark = ''
    resetOpen.value = false
    loaded.value = false
    pending.value = false
    error.value = ''
    status.value = 'unavailable'
    // Shared props update several session fields synchronously. Re-load once the
    // new permission/owner snapshot is complete, keeping late responses fenced.
    void nextTick().then(() => {
      if (alive && token === epoch && appearance.brandingRead) void load()
    })
  },
  { flush: 'sync' },
)
const unregisterRecovery = registerBrandingRecovery(() => {
  if (!loaded.value || !permission.value || !appearance.owner) return
  return {
    owner: appearance.owner,
    origin: location.origin,
    path: '/auth/profile/settings',
    created: Date.now(),
    version: version.value,
    name: draft.name,
    accentColor: draft.accentColor,
    accentColorEnd: draft.accentColorEnd,
    conflict: status.value === 'conflict',
    baseline: { ...baseline },
    ...(draft.logoLight === '' ? { logoLight: '' as const } : {}),
    ...(draft.logoDark === '' ? { logoDark: '' as const } : {}),
  }
})
onUnmounted(() => {
  unregisterRecovery()
  alive = false
  invalidateLogos()
  epoch++
  controller?.abort()
})
function hydrate(brand: ReturnType<typeof readBrand>) {
  baseline = {
    name: brand.name,
    accentColor: brand.accentColor,
    accentColorEnd: brand.accentColorEnd || '',
  }
  version.value = brand.version
  Object.assign(draft, {
    name: brand.name,
    accentColor: brand.accentColor,
    accentColorEnd: brand.accentColorEnd || '',
    logoLight: undefined,
    logoDark: undefined,
  })
  urls.light = brand.logoLightUrl
  urls.dark = brand.logoDarkUrl
}
async function load(keep = false) {
  if (!appearance.brandingRead) {
    status.value = 'unavailable'
    return
  }
  const token = ++epoch
  status.value = 'loading'
  error.value = ''
  invalidateLogos()
  controller?.abort()
  controller = new AbortController()
  try {
    if (!keep && permission.value && hasBrandingRecovery(appearance.owner)) {
      // Verify the current cookie's actor and rights, not potentially stale history props.
      const identity = await adminFetch('/auth/profile/settings', {
        signal: controller.signal,
        cache: 'no-store',
        redirect: 'error',
        headers: { Accept: 'text/html, application/xhtml+xml', 'X-Inertia': 'true' },
      })
      if (!alive || token !== epoch) return
      if (
        !identity.ok ||
        identity.redirected ||
        identity.headers.get('x-inertia') !== 'true' ||
        !identity.headers.get('content-type')?.toLowerCase().includes('application/json')
      )
        throw new Error('Identity unavailable')
      const page = await identity.json()
      if (!alive || token !== epoch) return
      const props = page?.props
      const actor = props?.authuser?.idExact ?? props?.authuser?.id
      if (
        page?.component !== 'profile/SettingsPage' ||
        actor == null ||
        String(actor) !== appearance.owner ||
        props?.appearance?.owner !== appearance.owner ||
        props?.appearance?.unavailable === true ||
        props?.appearanceAccess?.readOnly !== false ||
        props?.appearanceAccess?.brandingRead !== true ||
        props?.appearanceAccess?.brandingUpdate !== true
      ) {
        clearBrandingRecovery()
        throw new Error('Identity or permission changed')
      }
    }
    const response = await adminFetch('/auth/branding', {
      signal: controller.signal,
      headers: { Accept: 'application/json' },
    })
    if (!response.ok) throw new AppearanceSaveError(response.status)
    const doc = await response.json()
    if (!alive || token !== epoch) return
    if (!Number.isSafeInteger(doc.version) || doc.version < 0)
      throw new Error('Invalid branding version')
    const brand = readBrand({ version: doc.version, ...doc.payload })
    let recoveryConflict = false
    if (keep) {
      version.value = brand.version
      if (draft.name === baseline.name) draft.name = brand.name
      if (
        draft.accentColor === baseline.accentColor &&
        draft.accentColorEnd === baseline.accentColorEnd
      ) {
        draft.accentColor = brand.accentColor
        draft.accentColorEnd = brand.accentColorEnd || ''
      }
      if (draft.logoLight === undefined) urls.light = brand.logoLightUrl
      if (draft.logoDark === undefined) urls.dark = brand.logoDarkUrl
      baseline = {
        name: brand.name,
        accentColor: brand.accentColor,
        accentColorEnd: brand.accentColorEnd || '',
      }
    } else {
      hydrate(brand)
      const recovery = permission.value ? takeBrandingRecovery(appearance.owner) : undefined
      if (recovery) {
        recoveryConflict = recovery.conflict === true || recovery.version !== brand.version
        // Keep the rejected version until the user explicitly accepts a fresh one.
        if (recoveryConflict) version.value = recovery.version
        if (recovery.name !== recovery.baseline.name) draft.name = recovery.name
        if (
          recovery.accentColor !== recovery.baseline.accentColor ||
          recovery.accentColorEnd !== recovery.baseline.accentColorEnd
        ) {
          draft.accentColor = recovery.accentColor
          draft.accentColorEnd = recovery.accentColorEnd || ''
        }
        if (recovery.logoLight === '') {
          draft.logoLight = ''
          urls.light = ''
        }
        if (recovery.logoDark === '') {
          draft.logoDark = ''
          urls.dark = ''
        }
      }
    }
    loaded.value = true
    status.value = recoveryConflict ? 'conflict' : 'ready'
    error.value = ''
  } catch {
    if (alive && token === epoch) {
      status.value = 'error'
      error.value = 'Не удалось загрузить бренд.'
    }
  }
}
onMounted(() => load())
async function logo(variant: 'light' | 'dark', event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !permission.value) return
  const token = epoch
  const sequence = ++logoSequence[variant]
  decoding[variant] = true
  const current = () => alive && token === epoch && sequence === logoSequence[variant]
  try {
    if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 512 * 1024)
      throw new Error()
    const data = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(String(reader.result))
      reader.onerror = reject
      reader.readAsDataURL(file)
    })
    await new Promise<void>((resolve, reject) => {
      const image = new Image()
      image.onload = () =>
        image.width > 0 && image.height > 0 && image.width <= 1024 && image.height <= 1024
          ? resolve()
          : reject()
      image.onerror = reject
      image.src = data
    })
    if (!current()) return
    draft[variant === 'light' ? 'logoLight' : 'logoDark'] = data.slice(data.indexOf(',') + 1)
    urls[variant] = data
    error.value = ''
  } catch {
    if (current()) error.value = 'Выберите PNG или JPEG до 512 КиБ и 1024×1024.'
  } finally {
    if (current()) decoding[variant] = false
  }
}
function clearLogo(variant: 'light' | 'dark') {
  logoSequence[variant]++
  decoding[variant] = false
  draft[variant === 'light' ? 'logoLight' : 'logoDark'] = ''
  urls[variant] = ''
}
function confirmReset() {
  resetOpen.value = false
  void apply(true)
}
async function apply(reset = false) {
  if (
    decodingLogo.value ||
    !loaded.value ||
    !permission.value ||
    ['saving', 'conflict', 'loading', 'unavailable'].includes(status.value)
  )
    return
  if (
    !reset &&
    ((draft.accentColor && !/^#[\da-f]{6}$/i.test(draft.accentColor)) ||
      (draft.accentColorEnd &&
        (!draft.accentColor || !/^#[\da-f]{6}$/i.test(draft.accentColorEnd))))
  ) {
    error.value = 'Укажите HEX-цвет в формате #RRGGBB.'
    return
  }
  const token = ++epoch
  invalidateLogos()
  status.value = 'saving'
  error.value = ''
  controller?.abort()
  controller = new AbortController()
  try {
    const response = await adminFetch('/auth/branding', {
      method: 'PUT',
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({ version: version.value, ...draft, reset }),
    })
    if (!alive || token !== epoch) return
    if (response.status === 409) {
      status.value = 'conflict'
      return
    }
    if (!response.ok) throw new AppearanceSaveError(response.status)
    const doc = await response.json()
    if (!alive || token !== epoch) return
    const brand = readBrand({ version: doc.version, ...doc.payload })
    hydrate(brand)
    pending.value = doc.pending === true
    if (!pending.value && brand.version >= appearance.brand.version) {
      appearance.brand = brand
      paintAppearance()
    }
    status.value = 'saved'
  } catch (cause) {
    if (alive && token === epoch) {
      status.value = 'error'
      error.value = notifySaveFailure(cause)
    }
  }
}
</script>
<template>
  <section
    aria-labelledby="branding-title"
    class="min-w-0 space-y-6 rounded-xl border border-border-primary bg-card p-6"
  >
    <div>
      <h2 id="branding-title" class="text-xl font-semibold">
        {{ t('Бренд админки') }}
      </h2>
      <p class="mt-2 text-sm text-text-secondary">
        {{
          t(
            'Общие название, логотипы и акцент для всех пользователей. Предпросмотр виден только вам.',
          )
        }}
      </p>
    </div>
    <p v-if="status === 'loading'" role="status">{{ t('Загружаем...') }}</p>
    <fieldset
      :disabled="!loaded || !permission || ['loading', 'saving', 'unavailable'].includes(status)"
      class="min-w-0 space-y-4"
    >
      <label class="flex flex-col gap-2 text-sm font-medium"
        >{{ t('Название админки')
        }}<input
          v-model="draft.name"
          maxlength="80"
          class="appearance-select rounded-lg border border-border-primary bg-card px-3"
      /></label>
      <label class="flex flex-col gap-2 text-sm font-medium"
        >{{ t('Акцентный цвет')
        }}<input
          v-model="draft.accentColor"
          placeholder="#RRGGBB"
          maxlength="7"
          class="appearance-select rounded-lg border border-border-primary bg-card px-3"
      /></label>
      <div class="flex flex-wrap items-center gap-3">
        <label class="flex items-center gap-2 text-sm font-medium">
          {{ t('Выбрать акцентный цвет') }}
          <input type="color" :value="pickerColor" @input="setPickerColor" />
        </label>
        <AppButton variant="secondary" size="sm" @click="resetColors">{{
          t('Сбросить цвет')
        }}</AppButton>
      </div>
      <label class="flex flex-col gap-2 text-sm font-medium"
        >{{ t('Тип акцента') }}
        <select
          :value="draft.accentColorEnd ? 'gradient' : 'solid'"
          class="appearance-select rounded-lg border border-border-primary bg-card px-3"
          @change="setAccentMode"
        >
          <option value="solid">{{ t('Сплошной цвет') }}</option>
          <option value="gradient">{{ t('Градиент') }}</option>
        </select>
      </label>
      <label class="flex flex-col gap-2 text-sm font-medium"
        >{{ t('Конечный цвет градиента') }}
        <input
          v-model="draft.accentColorEnd"
          placeholder="#RRGGBB"
          maxlength="7"
          class="appearance-select rounded-lg border border-border-primary bg-card px-3"
        />
      </label>
      <label class="flex items-center gap-2 text-sm font-medium"
        >{{ t('Выбрать конечный цвет') }}
        <input
          type="color"
          :value="
            /^#[\da-f]{6}$/i.test(draft.accentColorEnd)
              ? draft.accentColorEnd
              : pickerColor || '#2563eb'
          "
          @input="setEndPickerColor"
        />
      </label>
      <p class="text-sm text-text-secondary">
        {{ t('Градиент 135°. Цвета заливки адаптируются для читаемости.') }}
      </p>
      <div v-for="variant in ['light', 'dark'] as const" :key="variant" class="space-y-2">
        <label class="flex flex-col gap-2 text-sm font-medium"
          >{{ t(variant === 'light' ? 'Логотип для светлой темы' : 'Логотип для тёмной темы')
          }}<input
            type="file"
            accept="image/png,image/jpeg"
            @change="logo(variant, $event)" /></label
        ><AppButton variant="secondary" size="sm" @click="clearLogo(variant)">{{
          t('Удалить логотип')
        }}</AppButton>
      </div>
      <p class="text-sm text-text-secondary">PNG / JPEG · ≤512 {{ t('КиБ') }} · ≤1024×1024</p>
    </fieldset>
    <div class="grid min-w-0 gap-4 sm:grid-cols-2" :aria-label="t('Предпросмотр')">
      <div
        v-for="variant in ['light', 'dark'] as const"
        :key="variant"
        :class="variant === 'dark' ? 'dark bg-[#20242b] text-white' : 'bg-white text-[#20242b]'"
        :style="deriveBrandPalette(draft.accentColor, variant === 'dark', draft.accentColorEnd)"
        class="min-w-0 space-y-4 rounded-xl border border-border-primary p-4"
      >
        <span class="text-xs">{{ t(variant === 'dark' ? 'Тёмная' : 'Светлая') }}</span>
        <div class="flex min-w-0 items-center gap-3">
          <img
            v-if="urls[variant]"
            :src="urls[variant]"
            alt=""
            class="h-10 w-10 shrink-0 object-contain"
          /><span class="min-w-0 [overflow-wrap:anywhere] font-semibold">{{
            draft.name || t('Админка')
          }}</span>
        </div>
        <span
          class="goadmin-primary-fill inline-flex rounded-lg bg-primary px-3 py-2 text-sm text-primary-contrast"
          >{{ t('Пример кнопки') }}</span
        >
        <label class="flex flex-col gap-2 text-sm"
          >{{ t('Пример поля')
          }}<input
            :value="t('Пример значения')"
            readonly
            class="appearance-select rounded-lg border border-current/30 bg-transparent px-3 text-inherit"
        /></label>
        <a
          href="#branding"
          class="inline-flex min-h-11 items-center text-sm text-primary underline underline-offset-2"
          @click.prevent
          >{{ t('Пример ссылки') }}</a
        >
      </div>
    </div>
    <p v-if="decodingLogo" role="status">{{ t('Проверяем логотип...') }}</p>
    <p v-if="error" role="alert">{{ t(error) }}</p>
    <AppButton v-if="!loaded && status === 'error'" variant="secondary" @click="load()">{{
      t('Повторить')
    }}</AppButton>
    <div v-if="status === 'conflict'" role="alert" class="space-y-3">
      <p>{{ t('Бренд изменён в другом окне. Черновик сохранён.') }}</p>
      <div class="flex flex-wrap gap-2">
        <AppButton :disabled="!permission" @click="load(true)">{{
          t('Получить новую версию для применения черновика')
        }}</AppButton
        ><AppButton variant="secondary" @click="load(false)">{{
          t('Загрузить сохранённое')
        }}</AppButton>
      </div>
    </div>
    <p v-if="status === 'saved'" role="status">
      {{
        t(
          pending
            ? 'Сохранено. Применение бренда ожидается; обновите страницу позже.'
            : 'Бренд сохранён и применён.',
        )
      }}
    </p>
    <div class="flex flex-wrap gap-3">
      <AppButton
        :disabled="
          decodingLogo ||
          !loaded ||
          !permission ||
          ['loading', 'saving', 'conflict', 'unavailable'].includes(status)
        "
        @click="apply()"
        >{{ t('Применить') }}</AppButton
      ><AppButton
        variant="secondary"
        :disabled="
          decodingLogo ||
          !loaded ||
          !permission ||
          ['loading', 'saving', 'conflict', 'unavailable'].includes(status)
        "
        @click="resetOpen = true"
        >{{ t('Сбросить бренд') }}</AppButton
      >
    </div>
    <Dialog v-model:open="resetOpen">
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{{ t('Сбросить бренд для всех пользователей?') }}</DialogTitle>
          <DialogDescription>{{
            t(
              'Название, логотипы и акцент будут заменены настройками по умолчанию для всей админки. Черновик будет удалён после успешного сброса.',
            )
          }}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <AppButton variant="secondary" @click="resetOpen = false">{{ t('Отмена') }}</AppButton>
          <AppButton :disabled="!permission || decodingLogo" @click="confirmReset">{{
            t('Подтвердить сброс')
          }}</AppButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>
</template>
