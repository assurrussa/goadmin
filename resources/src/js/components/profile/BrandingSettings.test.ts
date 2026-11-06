import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import BrandingSettings from './BrandingSettings.vue'
import { appearance, syncAppearance, disposeAppearance, pauseAppearance } from '@/appearance/state'
import { emptyBrand } from '@/appearance/brand'
const fetch = vi.hoisted(() => vi.fn())
const notifyError = vi.hoisted(() => vi.fn())
vi.mock('@/appearance/brand', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/appearance/brand')>()),
  themePrimaryHex: (dark: boolean) => (dark ? '#aabbcc' : '#112233'),
}))
vi.mock('@/composables/useNotifications', () => ({
  useNotifications: () => ({ error: notifyError }),
}))
vi.mock('@/http', () => ({
  adminFetch: (...args: unknown[]) => fetch(...args),
}))
let app: ReturnType<typeof createApp> | undefined
const brand = (name = 'Live', color = '#2563eb', version = 1, pending = false) => ({
  version,
  payload: { name, accentColor: color, logoLightUrl: '', logoDarkUrl: '' },
  pending,
})
const result = (data: unknown, status = 200) => ({
  ok: status < 400,
  status,
  json: async () => data,
})
const verified = (owner = '1', update = true) => ({
  ok: true,
  redirected: false,
  headers: new Headers({ 'X-Inertia': 'true', 'Content-Type': 'application/json' }),
  json: async () => ({
    component: 'profile/SettingsPage',
    props: {
      authuser: { idExact: owner },
      appearance: { owner },
      appearanceAccess: { readOnly: false, brandingRead: true, brandingUpdate: update },
    },
  }),
})
const flush = async () => {
  for (let i = 0; i < 10; i++) await Promise.resolve()
  await nextTick()
}
async function mount() {
  const root = document.createElement('div')
  app = createApp(BrandingSettings)
  document.body.append(root)
  app.mount(root)
  await flush()
  return root
}
function unmountApp() {
  app?.unmount()
  app = undefined
}
function input(root: HTMLElement, index: number, value: string) {
  const field = root.querySelectorAll('input')[index]!
  field.value = value
  field.dispatchEvent(new Event('input', { bubbles: true }))
}
function click(root: HTMLElement, text: string) {
  const button = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === text,
  )!
  button.click()
}
beforeEach(() => {
  fetch.mockReset()
  notifyError.mockReset()
  appearance.brand = { ...emptyBrand }
  syncAppearance({
    props: {
      authuser: { id: '1' },
      appearance: {
        owner: '1',
        version: 0,
        payload: {
          schemaVersion: 1,
          theme: 'light',
          locale: 'ru',
          density: 'default',
        },
      },
      branding: { version: 1, name: 'Live', accentColor: '#2563eb' },
      appearanceAccess: { brandingRead: true, brandingUpdate: true },
    },
  })
  fetch.mockResolvedValueOnce(result(brand()))
})
afterEach(() => {
  app?.unmount()
  app = undefined
  disposeAppearance()
  vi.unstubAllGlobals()
  sessionStorage.clear()
  document.body.replaceChildren()
})
it.each([401, 403, 419])(
  'shows %s save failure, keeps the brand draft and never replays',
  async (status) => {
    const root = await mount()
    input(root, 0, 'Unsaved brand')
    fetch.mockResolvedValueOnce(result({}, status))
    click(root, 'Применить')
    await flush()
    expect((root.querySelector('input') as HTMLInputElement).value).toBe('Unsaved brand')
    expect(appearance.brand.name).toBe('Live')
    expect(root.textContent).toContain(status === 403 ? 'Нет доступа' : 'Сеанс или защита')
    expect(notifyError).toHaveBeenCalledTimes(1)
    expect(notifyError.mock.calls[0]?.[0]).toContain('Изменения остаются на странице.')
    await flush()
    expect(fetch).toHaveBeenCalledTimes(2) // initial read and exactly one PUT
  },
)

it('keeps name and accent preview isolated until explicit successful Apply', async () => {
  const root = await mount()
  const accent = document.documentElement.style.getPropertyValue('--color-primary')
  input(root, 0, 'Preview')
  input(root, 1, '#eeddff')
  await nextTick()
  expect(root.textContent).toContain('Preview')
  expect(appearance.brand.name).toBe('Live')
  expect(document.documentElement.style.getPropertyValue('--color-primary')).toBe(accent)
  fetch.mockResolvedValueOnce(result(brand('Preview', '#eeddff', 2)))
  click(root, 'Применить')
  await flush()
  expect(appearance.brand.name).toBe('Preview')
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).toEqual({
    version: 1,
    name: 'Preview',
    accentColor: '#eeddff',
    accentColorEnd: '',
    reset: false,
  })
})

it('labels the accent mode without including its option text', async () => {
  fetch.mockResolvedValueOnce(result(brand()))
  const root = await mount()
  const select = root.querySelector('select')!
  expect([...select.labels!].map((label) => label.textContent?.trim())).toEqual(['Тип акцента'])
})
it('retains conflicting draft and merges untouched fresh fields before explicit reapply', async () => {
  const root = await mount()
  input(root, 0, 'Draft')
  fetch.mockResolvedValueOnce(result({}, 409))
  click(root, 'Применить')
  await flush()
  expect(root.textContent).toContain('Черновик сохранён')
  fetch.mockResolvedValueOnce(result(brand('Other', '#113355', 5)))
  click(root, 'Получить новую версию для применения черновика')
  await flush()
  expect(root.querySelector('input')!.value).toBe('Draft')
  fetch.mockResolvedValueOnce(result(brand('Draft', '#113355', 6)))
  click(root, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls[3]![1].body)).toMatchObject({
    version: 5,
    name: 'Draft',
    accentColor: '#113355',
  })
})
it('shows committed pending notice while retaining the last applied shared brand', async () => {
  const root = await mount()
  input(root, 0, 'Pending')
  fetch.mockResolvedValueOnce(result(brand('Pending', '#2563eb', 2, true)))
  click(root, 'Применить')
  await flush()
  expect(root.textContent).toContain('Сохранено. Применение бренда ожидается')
  expect(appearance.brand.name).toBe('Live')
})
it('aborts and ignores late application after session owner changes', async () => {
  const root = await mount()
  input(root, 0, 'Private draft')
  let finish!: (data: unknown) => void
  fetch.mockReturnValueOnce(
    new Promise((resolve) => {
      finish = resolve
    }),
  )
  click(root, 'Применить')
  await nextTick()
  const signal = fetch.mock.calls[1]![1].signal
  appearance.owner = '2'
  expect(signal.aborted).toBe(true)
  finish(result(brand('Private draft', '#2563eb', 2)))
  await flush()
  expect(appearance.brand.name).toBe('Live')
  expect(root.textContent).not.toContain('Private draft')
})
it('sends explicit empty logo for removal and reset through the same CAS API', async () => {
  const root = await mount()
  click(root, 'Удалить логотип')
  fetch.mockResolvedValueOnce(result(brand('', '', 2)))
  click(root, 'Сбросить бренд')
  await flush()
  click(document.body, 'Подтвердить сброс')
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).toMatchObject({
    version: 1,
    logoLight: '',
    reset: true,
  })
})

function deferredLogos(root: HTMLElement) {
  const readers: Array<{ result: string; onload: (() => void) | null }> = []
  const images: Array<{
    width: number
    height: number
    onload: (() => void) | null
    src: string
  }> = []
  vi.stubGlobal(
    'FileReader',
    class {
      result = ''
      onload: (() => void) | null = null
      onerror = null
      readAsDataURL() {
        readers.push(this)
      }
    },
  )
  vi.stubGlobal(
    'Image',
    class {
      width = 100
      height = 100
      onload: (() => void) | null = null
      onerror = null
      src = ''
      constructor() {
        images.push(this)
      }
    },
  )
  return {
    readers,
    images,
    select: () => {
      const field = root.querySelector<HTMLInputElement>('input[type=file]')!
      Object.defineProperty(field, 'files', {
        configurable: true,
        value: [new File(['png'], 'logo.png', { type: 'image/png' })],
      })
      field.dispatchEvent(new Event('change', { bubbles: true }))
    },
    read: async (index: number, data: string) => {
      readers[index]!.result = 'data:image/png;base64,' + data
      readers[index]!.onload?.()
      await flush()
    },
    decode: async (index: number) => {
      images[index]!.onload?.()
      await flush()
    },
  }
}
it('blocks Apply through file read and image decode, then sends raw base64', async () => {
  const root = await mount()
  const files = deferredLogos(root)
  files.select()
  await nextTick()
  const apply = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Применить',
  )!
  expect(apply.disabled).toBe(true)
  apply.click()
  expect(fetch).toHaveBeenCalledTimes(1)
  await files.read(0, 'aGVsbG8=')
  expect(apply.disabled).toBe(true)
  await files.decode(0)
  expect(apply.disabled).toBe(false)
  fetch.mockResolvedValueOnce(result(brand('Live', '#2563eb', 2)))
  apply.click()
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body).logoLight).toBe('aGVsbG8=')
})
it('keeps the newest logo when earlier image decode completes last', async () => {
  const root = await mount()
  const files = deferredLogos(root)
  files.select()
  await files.read(0, 'b2xk')
  files.select()
  await files.read(1, 'bmV3')
  await files.decode(1)
  await files.decode(0)
  fetch.mockResolvedValueOnce(result(brand('Live', '#2563eb', 2)))
  click(root, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body).logoLight).toBe('bmV3')
})
it('does not restore a cleared logo when its outstanding image decode completes', async () => {
  const root = await mount()
  const files = deferredLogos(root)
  files.select()
  await files.read(0, 'b2xk')
  click(root, 'Удалить логотип')
  await files.decode(0)
  expect(root.querySelectorAll('img')).toHaveLength(0)
  fetch.mockResolvedValueOnce(result(brand('Live', '#2563eb', 2)))
  click(root, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body).logoLight).toBe('')
})

it('loads permitted branding in read-only role preview while disabling mutations', async () => {
  appearance.readOnly = true
  const root = await mount()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(root.textContent).not.toContain('Загружаем...')
  expect(root.querySelector('input')!.value).toBe('Live')
  const apply = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Применить',
  )!
  expect(apply.disabled).toBe(true)
  apply.click()
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('cancels installation-wide reset without losing draft or sending HTTP', async () => {
  const root = await mount()
  input(root, 0, 'Keep this draft')
  click(root, 'Сбросить бренд')
  await flush()
  expect(document.body.textContent).toContain('Сбросить бренд для всех пользователей?')
  expect(fetch).toHaveBeenCalledTimes(1)
  click(document.body, 'Отмена')
  await flush()
  expect(root.querySelector('input')!.value).toBe('Keep this draft')
  expect(fetch).toHaveBeenCalledTimes(1)
})
it('previews field, link and button in both themes without navigating or mutating root tokens', async () => {
  const root = await mount()
  const before = document.documentElement.style.cssText
  const previews = root.querySelector('[aria-label="Предпросмотр"]')!
  expect(previews.querySelectorAll('input[readonly]')).toHaveLength(2)
  expect(previews.querySelectorAll('a')).toHaveLength(2)
  expect(previews.textContent?.match(/Пример кнопки/g)).toHaveLength(2)
  const event = new MouseEvent('click', { bubbles: true, cancelable: true })
  previews.querySelector('a')!.dispatchEvent(event)
  expect(event.defaultPrevented).toBe(true)
  expect(document.documentElement.style.cssText).toBe(before)
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('retains applied branding across stale history props and accepts a newer publication', async () => {
  const root = await mount()
  input(root, 0, 'Fresh')
  fetch.mockResolvedValueOnce(result(brand('Fresh', '#113355', 2)))
  click(root, 'Применить')
  await flush()
  const color = document.documentElement.style.getPropertyValue('--color-primary')
  const shared = (version: number, name: string) => ({
    props: {
      authuser: { id: '1' },
      appearance: { owner: '1', version: 0, payload: appearance.saved },
      branding: { version, name, accentColor: '#2563eb' },
      appearanceAccess: { brandingRead: true, brandingUpdate: true },
    },
  })
  syncAppearance(shared(1, 'Old'))
  expect(appearance.brand.name).toBe('Fresh')
  expect(appearance.brand.version).toBe(2)
  expect(document.documentElement.style.getPropertyValue('--color-primary')).toBe(color)
  syncAppearance(shared(3, 'New publication'))
  expect(appearance.brand.name).toBe('New publication')
})
it('isolates default preview accents by theme and leaves unset live accents to the CSS theme pack', async () => {
  const root = await mount()
  input(root, 1, '')
  await nextTick()
  const panels = root.querySelector('[aria-label="Предпросмотр"]')!.children
  expect((panels[0] as HTMLElement).style.getPropertyValue('--color-primary')).toBe(
    'hsl(217 69% 46%)',
  )
  expect((panels[1] as HTMLElement).style.getPropertyValue('--color-primary')).toBe(
    'hsl(217 69% 74%)',
  )
  fetch.mockResolvedValueOnce(result(brand('Live', '', 2)))
  click(root, 'Применить')
  await flush()
  expect(document.documentElement.style.getPropertyValue('--color-primary')).toBe('')
})

it('does not replace a newer shared brand when an older successful PUT body resolves late', async () => {
  const root = await mount()
  input(root, 0, 'Saved version two')
  let finish!: (value: unknown) => void
  fetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  })
  click(root, 'Применить')
  await flush()
  syncAppearance({
    props: {
      authuser: { id: '1' },
      appearance: { owner: '1', version: 0, payload: appearance.saved },
      appearanceAccess: { brandingRead: true, brandingUpdate: true },
      branding: { version: 3, name: 'Newest', accentColor: '#113355' },
    },
  })
  const liveColor = document.documentElement.style.getPropertyValue('--color-primary')
  finish(brand('Saved version two', '#2563eb', 2))
  await flush()
  expect(appearance.brand).toMatchObject({ version: 3, name: 'Newest' })
  expect(document.documentElement.style.getPropertyValue('--color-primary')).toBe(liveColor)
})
it('reloads a kept-mounted editor through a read-only preview roundtrip and permits writes only after exit', async () => {
  const root = await mount()
  fetch.mockResolvedValueOnce(result(brand('Preview snapshot', '#113355', 2)))
  appearance.readOnly = true
  await flush()
  expect(root.querySelector('input')!.value).toBe('Preview snapshot')
  const apply = [...root.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Применить',
  )!
  expect(apply.disabled).toBe(true)
  apply.click()
  expect(fetch).toHaveBeenCalledTimes(2)
  fetch.mockResolvedValueOnce(result(brand('Editable snapshot', '#113355', 3)))
  appearance.readOnly = false
  await flush()
  expect(root.querySelector('input')!.value).toBe('Editable snapshot')
  expect(apply.disabled).toBe(false)
  input(root, 0, 'After preview')
  fetch.mockResolvedValueOnce(result(brand('After preview', '#113355', 4)))
  apply.click()
  await flush()
  expect(JSON.parse(fetch.mock.calls[3]![1].body)).toMatchObject({
    version: 3,
    name: 'After preview',
  })
})
it('fences late branding GET bodies across an owner transition while reloading the mounted editor', async () => {
  const root = await mount()
  let finish!: (value: unknown) => void
  fetch.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  })
  appearance.readOnly = true
  await flush()
  const oldSignal = fetch.mock.calls[1]![1].signal
  fetch.mockResolvedValueOnce(result(brand('New owner snapshot', '#113355', 3)))
  appearance.owner = '2'
  await flush()
  expect(oldSignal.aborted).toBe(true)
  expect(root.querySelector('input')!.value).toBe('New owner snapshot')
  finish(brand('Old private response', '#2563eb', 2))
  await flush()
  expect(root.querySelector('input')!.value).toBe('New owner snapshot')
  expect(root.textContent).not.toContain('Old private response')
})

it('shows the theme primary without saving it, tracks theme changes and syncs picker and HEX', async () => {
  fetch.mockReset()
  fetch.mockResolvedValueOnce(result(brand('Live', '')))
  const root = await mount()
  const hex = root.querySelectorAll<HTMLInputElement>('input')[1]!
  const picker = root.querySelector<HTMLInputElement>('input[type=color]')!
  expect(hex.value).toBe('')
  expect(picker.value).toBe('#112233')
  appearance.dark = true
  await nextTick()
  expect(picker.value).toBe('#aabbcc')
  picker.value = '#123456'
  picker.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  expect(hex.value).toBe('#123456')
  hex.value = '#abcdef'
  hex.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  expect(picker.value).toBe('#abcdef')
  hex.value = 'invalid'
  hex.dispatchEvent(new Event('input', { bubbles: true }))
  await nextTick()
  click(root, 'Применить')
  await flush()
  expect(root.textContent).toContain('Укажите HEX-цвет')
  expect(fetch).toHaveBeenCalledTimes(1)
})

it('resets only the accent draft and preserves name and logos on save', async () => {
  const root = await mount()
  input(root, 0, 'Keep title')
  click(root, 'Сбросить цвет')
  await nextTick()
  expect(root.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('')
  expect(root.querySelector<HTMLInputElement>('input[type=color]')!.value).toBe('#112233')
  fetch.mockResolvedValueOnce(result(brand('Keep title', '', 2)))
  click(root, 'Применить')
  await flush()
  const payload = JSON.parse(fetch.mock.calls[1]![1].body)
  expect(payload).toEqual({
    version: 1,
    name: 'Keep title',
    accentColor: '',
    accentColorEnd: '',
    reset: false,
  })
  expect(appearance.brand.accentColor).toBe('')
})

it('resetting the accent keeps a pending uploaded logo and edited name', async () => {
  const root = await mount()
  input(root, 0, 'Keep title')
  const files = deferredLogos(root)
  files.select()
  await files.read(0, 'cG5n')
  await files.decode(0)
  click(root, 'Сбросить цвет')
  fetch.mockResolvedValueOnce(result(brand('Keep title', '', 2)))
  click(root, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).toMatchObject({
    name: 'Keep title',
    accentColor: '',
    logoLight: 'cG5n',
    reset: false,
  })
})

function restoreActor(id: string, canUpdate = true) {
  syncAppearance({
    props: {
      authuser: { id },
      appearance: {
        owner: id,
        version: 1,
        payload: { schemaVersion: 1, theme: 'light', locale: 'ru', density: 'default' },
      },
      appearanceAccess: { brandingRead: true, brandingUpdate: canUpdate },
    },
  })
}

it('restores only the same actor draft against fresh branding without replaying a PUT', async () => {
  const root = await mount()
  input(root, 0, 'Unsaved title')
  input(root, 1, '#123456')
  click(root, 'Удалить логотип')
  pauseAppearance()
  unmountApp()
  root.remove()
  restoreActor('1')
  fetch.mockResolvedValueOnce(verified())
  fetch.mockResolvedValueOnce(result(brand('New server title', '#abcdef', 9)))
  const restored = await mount()
  expect(restored.querySelectorAll<HTMLInputElement>('input')[0]!.value).toBe('Unsaved title')
  expect(restored.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('#123456')
  expect(fetch.mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(0)
  expect(restored.textContent).toContain('Черновик сохранён')
  fetch.mockResolvedValueOnce(result(brand('New server title', '#abcdef', 9)))
  click(restored, 'Получить новую версию для применения черновика')
  await flush()
  fetch.mockResolvedValueOnce(result(brand('Unsaved title', '#123456', 10)))
  click(restored, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls.at(-1)![1].body)).toMatchObject({
    version: 9,
    name: 'Unsaved title',
    accentColor: '#123456',
    logoLight: '',
    reset: false,
  })
})

it.each([
  ['2', true],
  ['1', false],
])('discards draft for changed actor or revoked rights (%s, %s)', async (id, allowed) => {
  const root = await mount()
  input(root, 1, '#123456')
  pauseAppearance()
  unmountApp()
  root.remove()
  restoreActor(id, allowed)
  fetch.mockResolvedValueOnce(result(brand('Fresh', '#abcdef', 9)))
  const restored = await mount()
  expect(restored.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('#abcdef')
  expect(sessionStorage.length).toBe(0)
  expect(fetch.mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(0)
})

it('never stores uploaded logo bytes in recovery and keeps fresh server logos', async () => {
  const root = await mount()
  const files = deferredLogos(root)
  files.select()
  await files.read(0, 'cG5n')
  await files.decode(0)
  input(root, 1, '#123456')
  pauseAppearance()
  expect(JSON.stringify(sessionStorage)).not.toContain('cG5n')
  unmountApp()
  root.remove()
  restoreActor('1')
  fetch.mockResolvedValueOnce(verified())
  fetch.mockResolvedValueOnce(result(brand('Live', '', 1)))
  const restored = await mount()
  fetch.mockResolvedValueOnce(result(brand('Live', '#123456', 10)))
  click(restored, 'Применить')
  await flush()
  const payload = JSON.parse(fetch.mock.calls.at(-1)![1].body)
  expect(payload.accentColor).toBe('#123456')
  expect(payload).not.toHaveProperty('logoLight')
})

it('keeps a conflict unresolved across a second reauthentication', async () => {
  const root = await mount()
  input(root, 1, '#123456')
  pauseAppearance()
  unmountApp()
  root.remove()
  restoreActor('1')
  fetch.mockResolvedValueOnce(verified())
  fetch.mockResolvedValueOnce(result(brand('New server', '#abcdef', 9)))
  const first = await mount()
  expect(first.textContent).toContain('Черновик сохранён')
  pauseAppearance()
  unmountApp()
  first.remove()
  restoreActor('1')
  fetch.mockResolvedValueOnce(verified())
  fetch.mockResolvedValueOnce(result(brand('New server', '#abcdef', 9)))
  const second = await mount()
  expect(second.textContent).toContain('Черновик сохранён')
  expect(second.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('#123456')
  const apply = [...second.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Применить',
  )!
  expect(apply.disabled).toBe(true)
  click(second, 'Применить')
  expect(fetch.mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(0)
  fetch.mockResolvedValueOnce(result(brand('New server', '#abcdef', 9)))
  click(second, 'Получить новую версию для применения черновика')
  await flush()
  fetch.mockResolvedValueOnce(result(brand('New server', '#123456', 10)))
  click(second, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls.at(-1)![1].body)).toMatchObject({
    version: 9,
    accentColor: '#123456',
  })
})

it.each([
  ['2', true],
  ['1', false],
])('does not restore when fresh server identity or update rights differ', async (owner, update) => {
  const root = await mount()
  input(root, 1, '#123456')
  pauseAppearance()
  unmountApp()
  root.remove()
  restoreActor('1') // stale history props still claim the original actor and rights
  fetch.mockResolvedValueOnce(verified(owner, update))
  const restored = await mount()
  expect(restored.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('')
  expect(restored.textContent).toContain('Не удалось загрузить бренд')
  expect(sessionStorage.length).toBe(0)
  expect(fetch.mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(0)
})

it('does not restore until the identity probe succeeds, and never follows redirects', async () => {
  const root = await mount()
  input(root, 1, '#123456')
  pauseAppearance()
  unmountApp()
  root.remove()
  restoreActor('1')
  fetch.mockResolvedValueOnce({
    ok: false,
    redirected: false,
    headers: new Headers(),
    json: async () => ({}),
  })
  const restored = await mount()
  const [url, init] = fetch.mock.calls.at(-1)!
  expect(url).toBe('/auth/profile/settings')
  expect(init).toMatchObject({
    cache: 'no-store',
    redirect: 'error',
    headers: { 'X-Inertia': 'true' },
  })
  expect(restored.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('')
  expect(fetch.mock.calls.filter(([, options]) => options?.method === 'PUT')).toHaveLength(0)
  fetch.mockResolvedValueOnce(verified())
  fetch.mockResolvedValueOnce(result(brand('Live', '#2563eb', 1)))
  click(restored, 'Повторить')
  await flush()
  expect(restored.querySelectorAll<HTMLInputElement>('input')[1]!.value).toBe('#123456')
})

it('previews a gradient without publishing and color reset preserves the name draft', async () => {
  const root = await mount()
  input(root, 0, 'Keep name')
  expect(root.querySelectorAll('input[type=color]')).toHaveLength(1)
  const mode = root.querySelector('select')!
  mode.value = 'gradient'
  mode.dispatchEvent(new Event('change', { bubbles: true }))
  await flush()
  const end = root.querySelectorAll<HTMLInputElement>('input:not([type])')[2]!
  end.value = '#fefefe'
  end.dispatchEvent(new Event('input', { bubbles: true }))
  await flush()
  expect(root.querySelectorAll('input[type="color"]')).toHaveLength(2)
  expect(root.querySelector('[aria-label="Предпросмотр"] > div')!.getAttribute('style')).toContain(
    'linear-gradient(135deg in srgb',
  )
  expect(appearance.brand.accentColorEnd).toBe('')
  expect(fetch.mock.calls).toHaveLength(1)
  click(root, 'Сбросить цвет')
  await flush()
  fetch.mockResolvedValueOnce(result(brand('Keep name', '', 2)))
  click(root, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls.at(-1)![1].body)).toMatchObject({
    name: 'Keep name',
    accentColor: '',
    accentColorEnd: '',
  })
})
it('rebases and recovers the gradient pair atomically when only the end changed', async () => {
  const initial = brand('Live', '#123456', 1)
  Object.assign(initial.payload, { accentColorEnd: '#654321' })
  fetch.mockReset()
  fetch.mockResolvedValueOnce(result(initial))
  const root = await mount()
  const end = root.querySelectorAll<HTMLInputElement>('input:not([type])')[2]!
  end.value = '#abcdef'
  end.dispatchEvent(new Event('input', { bubbles: true }))
  await flush()
  fetch.mockResolvedValueOnce(result({}, 409))
  click(root, 'Применить')
  await flush()
  const fresh = brand('Fresh', '#aabbcc', 9)
  Object.assign(fresh.payload, { accentColorEnd: '#ddeeff' })
  fetch.mockResolvedValueOnce(result(fresh))
  click(root, 'Получить новую версию для применения черновика')
  await flush()
  pauseAppearance()
  unmountApp()
  root.remove()
  restoreActor('1')
  fetch.mockResolvedValueOnce(verified())
  fetch.mockResolvedValueOnce(result(fresh))
  const restored = await mount()
  fetch.mockResolvedValueOnce(result(brand('Fresh', '#123456', 10)))
  click(restored, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls.at(-1)![1].body)).toMatchObject({
    version: 9,
    name: 'Fresh',
    accentColor: '#123456',
    accentColorEnd: '#abcdef',
  })
})

it('retains a temporarily empty gradient field for correction and serializes Solid as an empty endpoint', async () => {
  const root = await mount()
  const mode = root.querySelector('select')!
  mode.value = 'gradient'
  mode.dispatchEvent(new Event('change', { bubbles: true }))
  await flush()
  const end = root.querySelectorAll<HTMLInputElement>('input:not([type])')[2]!
  end.value = ''
  end.dispatchEvent(new Event('input', { bubbles: true }))
  await flush()
  expect(mode.value).toBe('gradient')
  expect(root.querySelectorAll('input[type=color]')).toHaveLength(2)
  click(root, 'Применить')
  await flush()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(root.querySelector('[role=alert]')!.textContent).toContain('#RRGGBB')
  mode.value = 'solid'
  mode.dispatchEvent(new Event('change', { bubbles: true }))
  await flush()
  expect(root.querySelectorAll('input[type=color]')).toHaveLength(1)
  fetch.mockResolvedValueOnce(result(brand('Live', '#2563eb', 2)))
  click(root, 'Применить')
  await flush()
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).toMatchObject({
    accentColor: '#2563eb',
    accentColorEnd: '',
  })
})
