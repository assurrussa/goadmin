import { afterEach, expect, it, vi } from 'vitest'
import { createApp, h, nextTick, type Component } from 'vue'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
  DropdownMenuItem,
  DropdownMenuCheckboxItem,
  DropdownMenuRadioItem,
  DropdownMenuRadioGroup,
  DropdownMenuSub,
  DropdownMenuSubTrigger,
} from './dropdown-menu'
import { Select, SelectContent, SelectItem, SelectTrigger } from './select'
import { Tabs, TabsList, TabsTrigger } from './tabs'

const apps: ReturnType<typeof createApp>[] = []
afterEach(() => {
  apps.splice(0).forEach((app) => app.unmount())
})

const cases: [string, Component][] = [
  ['item', DropdownMenuItem],
  ['checkbox', DropdownMenuCheckboxItem],
  ['radio', DropdownMenuRadioItem],
  ['sub-trigger', DropdownMenuSubTrigger],
  ['select', SelectItem],
  ['tab', TabsTrigger],
]
for (const [name, component] of cases) {
  it.each([false, true])(
    `${name} fences disabled consumer activation (asChild=%s)`,
    async (asChild) => {
      const clicked = vi.fn()
      const keyed = vi.fn()
      const selected = vi.fn()
      const childClicked = vi.fn()
      const childKeyed = vi.fn()
      const child = () =>
        asChild
          ? h('a', { href: '#disabled', onClick: childClicked, onKeydown: childKeyed }, 'Item')
          : 'Item'
      const item = () =>
        h(
          component,
          {
            disabled: true,
            asChild,
            value: 'item',
            'data-test-control': '',
            onClick: clicked,
            onKeydown: keyed,
            onSelect: selected,
          },
          child,
        )
      const root = mount(name, item)
      await nextTick()
      await nextTick()
      const element =
        root.querySelector('[data-test-control]') ||
        document.body.querySelector('[data-test-control]')
      expect(element).not.toBeNull()
      if (asChild) expect(element!.tagName).toBe('A')
      const events = [
        new MouseEvent('click', { bubbles: true, cancelable: true }),
        ...['pointerdown', 'pointerup', 'pointerup'].map(
          (type) => new MouseEvent(type, { bubbles: true, cancelable: true }),
        ),
        ...['Enter', ' '].flatMap((key) =>
          ['keydown', 'keyup'].map(
            (type) => new KeyboardEvent(type, { key, bubbles: true, cancelable: true }),
          ),
        ),
      ]
      for (const event of events) {
        element!.dispatchEvent(event)
        expect(event.defaultPrevented).toBe(true)
      }
      await nextTick()
      await nextTick()
      for (const handler of [clicked, keyed, selected, childClicked, childKeyed])
        expect(handler).not.toHaveBeenCalled()
    },
  )
  it.each([false, true])(
    `${name} preserves enabled click handlers (asChild=%s)`,
    async (asChild) => {
      const clicked = vi.fn()
      const childClicked = vi.fn()
      const item = () =>
        h(
          component,
          {
            disabled: false,
            asChild,
            value: 'item',
            'data-test-control': '',
            onClick: clicked,
          },
          () => (asChild ? h('a', { href: '#enabled', onClick: childClicked }, 'Item') : 'Item'),
        )
      const root = mount(name, item)
      await nextTick()
      await nextTick()
      const element =
        root.querySelector('[data-test-control]') ||
        document.body.querySelector('[data-test-control]')
      expect(element).not.toBeNull()
      if (asChild) expect(element!.tagName).toBe('A')
      element!.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true }))
      expect(clicked).toHaveBeenCalledOnce()
      if (asChild) expect(childClicked).toHaveBeenCalledOnce()
    },
  )
}

function mount(name: string, item: () => ReturnType<typeof h>) {
  const root = document.createElement('div')
  const render = () => {
    if (name === 'tab') return h(Tabs, { defaultValue: 'other' }, () => h(TabsList, {}, item))
    if (name === 'select')
      return h(Select, { defaultOpen: true }, () => [
        h(SelectTrigger, {}, () => 'Open'),
        h(SelectContent, {}, item),
      ])
    const content =
      name === 'radio'
        ? () => h(DropdownMenuRadioGroup, {}, item)
        : name === 'sub-trigger'
          ? () => h(DropdownMenuSub, {}, item)
          : item
    return h(DropdownMenu, { defaultOpen: true }, () => [
      h(DropdownMenuTrigger, {}, () => 'Open'),
      h(DropdownMenuContent, {}, content),
    ])
  }
  const app = createApp({ render })
  apps.push(app)
  app.mount(root)
  return root
}

it.each([false, true])(
  'enabled SelectItem preserves pointerup selection (asChild=%s)',
  async (asChild) => {
    const selected = vi.fn((event: Event) => event.preventDefault())
    const pointerup = vi.fn()
    mount('select', () =>
      h(
        SelectItem,
        {
          disabled: false,
          asChild,
          value: 'item',
          onSelect: selected,
          onPointerup: pointerup,
          'data-test-control': '',
        },
        () => (asChild ? h('a', { href: '#item' }, 'Item') : 'Item'),
      ),
    )
    await nextTick()
    await nextTick()
    const element = document.body.querySelector('[data-test-control]')!
    // The content suppresses its initial opening pointerup; the next one selects.
    for (let i = 0; i < 2; i++) {
      element.dispatchEvent(new MouseEvent('pointerup', { bubbles: true, cancelable: true }))
      await nextTick()
      await nextTick()
    }
    expect(pointerup).toHaveBeenCalledTimes(2)
    expect(selected).toHaveBeenCalledOnce()
  },
)
