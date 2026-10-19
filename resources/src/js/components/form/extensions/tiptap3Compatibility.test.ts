import { describe, expect, it, vi } from 'vitest'
import { Editor, Node, mergeAttributes } from '@tiptap/core'
import { DOMSerializer } from '@tiptap/pm/model'
import StarterKit from '@tiptap/starter-kit'
import Image from '@tiptap/extension-image'
import Link from '@tiptap/extension-link'
import Underline from '@tiptap/extension-underline'
import { Table, TableCell, TableHeader, TableRow } from '@tiptap/extension-table'
import { TaskItem, TaskList } from '@tiptap/extension-list'
import { VideoExtension } from './Video'
import { CmsFileLink, CmsImage, CmsVideo } from './CmsMedia'

const extensions = () => [
  StarterKit.configure({ link: false, underline: false, listKeymap: false, trailingNode: false }),
  Link.configure({ openOnClick: false }),
  Underline,
  Image.configure({ allowBase64: true }),
  VideoExtension,
  CmsImage,
  CmsVideo,
  CmsFileLink,
  Table.configure({ resizable: true }),
  TableRow,
  TableHeader,
  TableCell,
  TaskList,
  TaskItem.configure({ nested: true }),
]

describe('patched Tiptap 3 compatibility', () => {
  it('keeps JSON-origin prototype attributes inert through a custom extension and DOMSerializer', () => {
    const hostile = JSON.parse(
      '{"__proto__":{"onerror":"canary()","data-inherited-canary":"present"}}',
    )
    const attrs = mergeAttributes({ class: 'host-image' }, hostile)
    expect(Object.getPrototypeOf(attrs)).toBe(Object.prototype)
    expect(attrs.onerror).toBeUndefined()
    const CustomImage = Node.create({
      name: 'hostImage',
      group: 'block',
      atom: true,
      addAttributes: () => ({ payload: { default: null, rendered: false } }),
      renderHTML: ({ node }) => [
        'img',
        mergeAttributes({ class: 'host-image' }, node.attrs.payload),
      ],
    })
    const editor = new Editor({
      extensions: [...extensions(), CustomImage],
      content: { type: 'doc', content: [{ type: 'hostImage', attrs: { payload: hostile } }] },
    })
    try {
      const output = document.createElement('div')
      output.append(
        DOMSerializer.fromSchema(editor.schema).serializeFragment(editor.state.doc.content),
      )
      expect(output.querySelector('img')?.className).toBe('host-image')
      expect(output.querySelector('[onerror], [data-inherited-canary]')).toBeNull()
      expect(Object.hasOwn(Object.prototype, 'onerror')).toBe(false)
    } finally {
      editor.destroy()
    }
  })

  it('round-trips formatting, tables, lists, links and all media without changing JSON', () => {
    const editor = new Editor({ extensions: extensions(), content: '<p>hello</p>' })
    try {
      expect(editor.commands.selectAll()).toBe(true)
      expect(
        editor
          .chain()
          .toggleBold()
          .toggleItalic()
          .toggleUnderline()
          .setLink({ href: 'https://example.com' })
          .run(),
      ).toBe(true)
      editor.commands.setTextSelection(editor.state.doc.content.size - 1)
      expect(editor.commands.setVideo({ src: '/movie.mp4', poster: '/poster.png' })).toBe(true)
      editor.commands.setTextSelection(editor.state.doc.content.size)
      expect(editor.commands.setImage({ src: 'data:image/png;base64,AA==', alt: 'image' })).toBe(
        true,
      )
      editor.commands.setTextSelection(editor.state.doc.content.size)
      expect(editor.commands.insertTable({ rows: 2, cols: 2, withHeaderRow: true })).toBe(true)
      editor.commands.setTextSelection(editor.state.doc.content.size)
      expect(
        editor.commands.insertContent([
          { type: 'cmsImage', attrs: { assetId: 'image', alt: 'alt', caption: 'caption' } },
          { type: 'cmsVideo', attrs: { assetId: 'video', provider: 'local' } },
          {
            type: 'paragraph',
            content: [
              {
                type: 'text',
                text: 'file',
                marks: [{ type: 'cmsFileLink', attrs: { assetId: 'file' } }],
              },
            ],
          },
          {
            type: 'taskList',
            content: [
              {
                type: 'taskItem',
                attrs: { checked: true },
                content: [{ type: 'paragraph', content: [{ type: 'text', text: 'task' }] }],
              },
            ],
          },
        ]),
      ).toBe(true)
      const json = editor.getJSON()
      const html = editor.getHTML()
      for (const tag of [
        '<strong>',
        '<em>',
        '<u>',
        '<a ',
        '<img ',
        '<video ',
        '<table ',
        'data-cms-image',
        'data-cms-video',
        'data-cms-file-link',
        'data-checked="true"',
      ]) {
        expect(html).toContain(tag)
      }
      const copy = new Editor({ extensions: extensions(), content: json, enableContentCheck: true })
      try {
        expect(copy.getJSON()).toEqual(json)
        expect(copy.getHTML()).toBe(html)
      } finally {
        copy.destroy()
      }
      expect(
        new Set(editor.extensionManager.extensions.map((extension) => extension.name)).size,
      ).toBe(editor.extensionManager.extensions.length)
    } finally {
      editor.destroy()
    }
  })

  it('preserves silent controlled updates, invalid-content rejection and undo/redo commands', () => {
    const onUpdate = vi.fn()
    const editor = new Editor({ extensions: extensions(), content: '<p>before</p>', onUpdate })
    try {
      editor.commands.setContent('<p>controlled</p>', {
        emitUpdate: false,
        errorOnInvalidContent: true,
      })
      expect(onUpdate).not.toHaveBeenCalled()
      expect(editor.getText()).toBe('controlled')
      expect(() =>
        editor.commands.setContent(
          { type: 'doc', content: [{ type: 'unknownHostNode' }] },
          { errorOnInvalidContent: true },
        ),
      ).toThrow()
      expect(editor.commands.insertContent('edit')).toBe(true)
      const changed = editor.getJSON()
      expect(editor.commands.undo()).toBe(true)
      expect(editor.getJSON()).not.toEqual(changed)
      expect(editor.commands.redo()).toBe(true)
      expect(editor.getJSON()).toEqual(changed)
    } finally {
      editor.destroy()
    }
  })
})
