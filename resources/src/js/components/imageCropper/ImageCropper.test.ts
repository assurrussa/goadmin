import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, ref } from 'vue'
import ImageCropper from './ImageCropper.vue'

const cropState = vi.hoisted(() => ({ complete: null as BlobCallback | null }))
vi.mock('cropperjs', () => ({
  default: class {
    destroy() {}
    setAspectRatio() {}
    getCroppedCanvas() {
      return { toBlob: (callback: BlobCallback) => (cropState.complete = callback) }
    }
  },
}))

const cleanups: Array<() => void> = []
afterEach(() => {
  cleanups.splice(0).forEach((cleanup) => cleanup())
  cropState.complete = null
})

function mountCropper() {
  const opener = document.createElement('button')
  opener.textContent = 'Open cropper'
  document.body.append(opener)
  opener.focus()
  const root = document.createElement('div')
  document.body.append(root)
  const open = ref(true)
  const onCrop = vi.fn()
  const app = createApp({
    setup: () => () =>
      open.value
        ? h(ImageCropper, {
            imageUrl: 'data:image/png;base64,synthetic',
            fileName: 'synthetic.png',
            onClose: () => (open.value = false),
            onCrop,
          })
        : null,
  })
  app.mount(root)
  cleanups.push(() => {
    app.unmount()
    opener.remove()
    root.remove()
  })
  return { opener, open, onCrop }
}

it('names the modal, moves focus inside, traps Tab and restores focus after Escape', async () => {
  const { opener, open } = mountCropper()
  await vi.waitFor(() => expect(document.activeElement?.closest('[role="dialog"]')).toBeTruthy())
  const dialog = document.querySelector('[role="dialog"]')!
  expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toBe(
    'Обрезать изображение',
  )
  const buttons = dialog.querySelectorAll('button')
  const first = buttons[0]
  const last = buttons[buttons.length - 1]
  first.focus()
  first.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true }))
  expect(document.activeElement).toBe(last)
  last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await vi.waitFor(() => {
    expect(open.value).toBe(false)
    expect(document.activeElement).toBe(opener)
  })
})

it('keeps the dialog open during crop processing and emits the resulting file', async () => {
  const { open, onCrop } = mountCropper()
  await vi.waitFor(() => expect(document.activeElement?.closest('[role="dialog"]')).toBeTruthy())
  const dialog = document.querySelector('[role="dialog"]')!
  const crop = Array.from(dialog.querySelectorAll('button')).find(
    (button) => button.textContent?.trim() === 'Обрезать',
  )!
  crop.click()
  await nextTick()
  expect(dialog.querySelector('[role="status"]')?.textContent).toContain('Готовим изображение')
  dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
  await nextTick()
  expect(open.value).toBe(true)
  cropState.complete!(new Blob(['synthetic'], { type: 'image/png' }))
  await nextTick()
  const file = onCrop.mock.calls[0][0] as File
  expect(file.name).toBe('synthetic.png')
  expect(file.type).toBe('image/png')
  expect(file.size).toBe(9)
})
