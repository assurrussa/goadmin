import { describe, expect, it } from 'vitest'
import { Editor, Node, mergeAttributes } from '@tiptap/core'
import { DOMSerializer } from '@tiptap/pm/model'
import StarterKit from '@tiptap/starter-kit'

const hostileAttributes = () =>
  JSON.parse(
    '{"__proto__":{"onerror":"canary()","onclick":"canary()","src":"x-invalid://canary","data-inherited-canary":"present"},"alt":"safe alt"}',
  ) as Record<string, unknown>

const inputs = (position: number) => {
  const objects = [
    { class: 'first', title: 'safe title' },
    { class: 'second', src: '/safe-image.png' },
  ]
  return [...objects.slice(0, position), hostileAttributes(), ...objects.slice(position)]
}

const expectSafePrototype = (attrs: Record<string, unknown>) => {
  expect(Object.getPrototypeOf(attrs)).toBe(Object.prototype)
  expect(Object.getOwnPropertyDescriptor(attrs, '__proto__')).toMatchObject({
    enumerable: true,
    writable: true,
    configurable: true,
    value: expect.objectContaining({ onerror: 'canary()' }),
  })
  expect(attrs.onerror).toBeUndefined()
  expect(attrs.onclick).toBeUndefined()
  expect(attrs['data-inherited-canary']).toBeUndefined()
  expect(Object.hasOwn(Object.prototype, 'onerror')).toBe(false)
  expect(Object.hasOwn(Object.prototype, 'data-inherited-canary')).toBe(false)
}

const expectSafeImage = (container: HTMLElement) => {
  const image = container.querySelector('img')
  expect(image).not.toBeNull()
  expect(image?.getAttribute('src')).toBe('/safe-image.png')
  expect(image?.getAttribute('alt')).toBe('safe alt')
  expect(image?.getAttribute('title')).toBe('safe title')
  expect(image?.className).toBe('first second')
  expect(container.querySelector('[onerror], [onclick], [data-inherited-canary]')).toBeNull()
}

describe('Tiptap mergeAttributes backport security', () => {
  it.each([0, 1, 2])(
    'retains the safe prototype with hostile input at merge position %i',
    (position) => {
      const attrs = mergeAttributes(...inputs(position))
      expectSafePrototype(attrs)
      // __proto__ is an inert own data property, not a dropped/sanitized key.
      expect(Object.keys(attrs)).toContain('__proto__')
      expect(attrs.src).toBe('/safe-image.png')
      expect(attrs.class).toBe('first second')
      expectSafePrototype(mergeAttributes(attrs, { title: 'updated' }))
    },
  )

  it.each([0, 1, 2])(
    'prevents inherited executable DOM attributes in a custom extension at position %i',
    (position) => {
      // Unlike the fixed first-party media schemas, this host-like extension
      // deliberately forwards an arbitrary JSON attribute map to the helper.
      const CustomImage = Node.create({
        name: 'hostImage',
        group: 'block',
        atom: true,
        addAttributes: () => ({ payload: { default: null, rendered: false } }),
        renderHTML: ({ node }) => {
          const objects = inputs(position)
          objects[position] = node.attrs.payload
          const attrs = mergeAttributes(...objects)
          expectSafePrototype(attrs)
          return ['img', attrs]
        },
      })
      const editor = new Editor({
        extensions: [StarterKit, CustomImage],
        content: {
          type: 'doc',
          content: [{ type: 'hostImage', attrs: { payload: hostileAttributes() } }],
        },
      })
      try {
        const serialized = document.createElement('div')
        serialized.append(
          DOMSerializer.fromSchema(editor.schema).serializeFragment(editor.state.doc.content),
        )
        expectSafeImage(serialized)
        const html = document.createElement('div')
        html.innerHTML = editor.getHTML()
        expectSafeImage(html)
        // ProseMirror can serialize the inert own key as an HTML attribute;
        // the required protection is no prototype mutation/inherited handlers.
        expect(editor.getJSON().content?.[0]?.attrs?.payload).toEqual(hostileAttributes())
      } finally {
        editor.destroy()
      }
    },
  )
})
