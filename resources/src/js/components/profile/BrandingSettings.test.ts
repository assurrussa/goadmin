import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import BrandingSettings from './BrandingSettings.vue'
import { appearance, syncAppearance, disposeAppearance } from '@/appearance/state'
import { emptyBrand } from '@/appearance/brand'
const fetch = vi.hoisted(() => vi.fn())
vi.mock('@/http', () => ({ adminFetch: (...args: unknown[]) => fetch(...args) }))
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
  appearance.brand = { ...emptyBrand }
  syncAppearance({
    props: {
      authuser: { id: '1' },
      appearance: {
        owner: '1',
        version: 0,
        payload: { schemaVersion: 1, theme: 'light', locale: 'ru', density: 'default' },
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
  document.body.replaceChildren()
})
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
    reset: false,
  })
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
  const images: Array<{ width: number; height: number; onload: (() => void) | null; src: string }> =
    []
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
