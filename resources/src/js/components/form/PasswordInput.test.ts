import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, type App } from 'vue'
import PasswordInput from './PasswordInput.vue'

let app: App | undefined
let container: HTMLDivElement | undefined

afterEach(() => {
  app?.unmount()
  container?.remove()
})

function mount(props: Record<string, unknown> = {}) {
  const submit = vi.fn((event: Event) => event.preventDefault())
  container = document.createElement('div')
  document.body.appendChild(container)
  app = createApp({
    render: () =>
      h('form', { onSubmit: submit }, [
        h(PasswordInput, { modelValue: 'Synthetic-password', label: 'Пароль', ...props }),
      ]),
  })
  app.mount(container)
  return {
    input: container.querySelector('input')!,
    button: container.querySelector('button')!,
    submit,
  }
}

describe('PasswordInput visibility control', () => {
  it('exposes the current action and pressed state without submitting or changing the password', async () => {
    const { input, button, submit } = mount()
    expect(button.type).toBe('button')
    expect(button.tabIndex).toBe(0)
    expect(button.getAttribute('aria-label')).toBe('Показать пароль')
    expect(button.getAttribute('aria-pressed')).toBe('false')
    expect(button.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
    expect(input.type).toBe('password')

    button.click()
    await nextTick()
    expect(input.type).toBe('text')
    expect(button.getAttribute('aria-label')).toBe('Скрыть пароль')
    expect(button.getAttribute('aria-pressed')).toBe('true')
    expect(button.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
    expect(input.value).toBe('Synthetic-password')
    expect(submit).not.toHaveBeenCalled()

    button.click()
    await nextTick()
    expect(input.type).toBe('password')
    expect(button.getAttribute('aria-label')).toBe('Показать пароль')
    expect(button.getAttribute('aria-pressed')).toBe('false')
  })

  it.each(['disabled', 'readonly'])(
    'preserves %s protection on the input and visibility control',
    async (prop) => {
      const { input, button, submit } = mount({ [prop]: true })
      expect(input.hasAttribute(prop)).toBe(true)
      expect(button.disabled).toBe(true)
      button.click()
      await nextTick()
      expect(input.type).toBe('password')
      expect(submit).not.toHaveBeenCalled()
    },
  )
})
