import { describe, expect, it } from 'vitest'
import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import Image from '@tiptap/extension-image'
import Link from '@tiptap/extension-link'
import { CmsFileLink, CmsImage, CmsVideo } from './CmsMedia'
import { VideoExtension } from './Video'

// Exercise the actual fixed media schemas used by RichTextEditor. An own
// JSON-origin __proto__ must never reach mergeAttributes as an attribute key.
const hostileAttributes = () =>
  JSON.parse(
    '{"__proto__":{"onerror":"canary()","onclick":"canary()","data-inherited-canary":"present"}}',
  ) as Record<string, unknown>

const createEditor = (content: object | string) =>
  new Editor({
    extensions: [
      StarterKit,
      Image.configure({ HTMLAttributes: { class: 'editor-image' } }),
      Link.configure({ openOnClick: false, HTMLAttributes: { class: 'link link-primary' } }),
      VideoExtension.configure({
        HTMLAttributes: { class: 'editor-video', controls: true, preload: 'metadata' },
      }),
      CmsImage,
      CmsVideo,
      CmsFileLink,
    ],
    content,
  })

const expectSafeSerialization = (editor: Editor) => {
  const output = document.createElement('div')
  output.innerHTML = editor.getHTML()
  expect(output.querySelectorAll('*').length).toBeGreaterThan(0)
  for (const element of output.querySelectorAll('*')) {
    for (const attribute of element.attributes) {
      expect(attribute.name).not.toMatch(/^on/i)
      expect(attribute.name).not.toBe('data-inherited-canary')
      expect(attribute.name).not.toBe('__proto__')
    }
  }
  expect(Object.hasOwn(Object.prototype, 'data-inherited-canary')).toBe(false)
}

describe('rich text media attribute boundary', () => {
  it.each(['image', 'video', 'cmsImage', 'cmsVideo'])(
    'drops own prototype keys from imported %s JSON and updates',
    (type) => {
      const content = {
        type: 'doc',
        content: [
          { type, attrs: { ...hostileAttributes(), src: '/test-image', assetId: 'asset-test' } },
        ],
      }
      const editor = createEditor(content)
      try {
        expectSafeSerialization(editor)
        expect(editor.getJSON().content?.some((node) => node.type === type)).toBe(true)
        editor.commands.setContent(content)
        expectSafeSerialization(editor)
      } finally {
        editor.destroy()
      }
    },
  )

  it.each(['link', 'cmsFileLink'])('drops own prototype keys from %s marks', (type) => {
    const editor = createEditor({
      type: 'doc',
      content: [
        {
          type: 'paragraph',
          content: [
            {
              type: 'text',
              text: 'A test link',
              marks: [{ type, attrs: { ...hostileAttributes(), href: '/test', assetId: 'asset' } }],
            },
          ],
        },
      ],
    })
    try {
      expectSafeSerialization(editor)
      expect(editor.getHTML()).toContain('<a ')
    } finally {
      editor.destroy()
    }
  })

  it('filters executable and prototype attributes when importing HTML', () => {
    const editor = createEditor(
      '<img src="/test" onerror="canary()" data-inherited-canary="present">' +
        '<video src="/test" onclick="canary()"></video>' +
        '<figure data-cms-image assetId="asset" onerror="canary()"></figure>' +
        '<figure data-cms-video assetId="asset" onclick="canary()"></figure>' +
        '<p><a data-cms-file-link onclick="canary()">Test link</a></p>',
    )
    try {
      expectSafeSerialization(editor)
      expect(editor.getHTML()).toContain('data-cms-image')
      expect(editor.getHTML()).toContain('data-cms-video')
      expect(editor.getHTML()).toContain('data-cms-file-link')
    } finally {
      editor.destroy()
    }
  })

  it('filters prototype keys from the video insertion command', () => {
    const editor = createEditor('<p>Test</p>')
    try {
      expect(editor.commands.setVideo({ ...hostileAttributes(), src: '/test-video' })).toBe(true)
      expectSafeSerialization(editor)
      expect(editor.getHTML()).toContain('<video ')
    } finally {
      editor.destroy()
    }
  })
})
