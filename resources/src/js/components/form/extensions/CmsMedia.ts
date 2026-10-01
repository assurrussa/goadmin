import { Mark, Node, mergeAttributes } from '@tiptap/core'

const mediaLabel = (kind: string, attrs: Record<string, unknown>) => {
  const id = attrs.assetId ?? attrs.externalUrl ?? attrs.id ?? 'unknown'
  return `${kind}: ${String(id)}`
}

export const CmsImage = Node.create({
  name: 'cmsImage',
  group: 'block',
  atom: true,
  draggable: true,

  addAttributes() {
    return {
      id: { default: null },
      assetId: { default: null },
      alt: { default: '' },
      caption: { default: '' },
    }
  },

  parseHTML() {
    return [{ tag: '[data-cms-image]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'figure',
      mergeAttributes(HTMLAttributes, {
        class: 'cms-media-node cms-image-node',
        'data-cms-image': '',
      }),
      mediaLabel('Image', HTMLAttributes),
    ]
  },
})

export const CmsVideo = Node.create({
  name: 'cmsVideo',
  group: 'block',
  atom: true,
  draggable: true,

  addAttributes() {
    return {
      id: { default: null },
      sourceType: { default: null },
      asset: { default: null },
      assetId: { default: null },
      provider: { default: null },
      externalUrl: { default: null },
      poster: { default: null },
    }
  },

  parseHTML() {
    return [{ tag: '[data-cms-video]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'figure',
      mergeAttributes(HTMLAttributes, {
        class: 'cms-media-node cms-video-node',
        'data-cms-video': '',
      }),
      mediaLabel('Video', HTMLAttributes),
    ]
  },
})

export const CmsFileLink = Mark.create({
  name: 'cmsFileLink',

  addAttributes() {
    return {
      id: { default: null },
      assetId: { default: null },
      label: { default: '' },
      download: { default: false },
    }
  },

  parseHTML() {
    return [{ tag: 'a[data-cms-file-link]' }]
  },

  renderHTML({ HTMLAttributes }) {
    return [
      'a',
      mergeAttributes(HTMLAttributes, {
        class: 'cms-file-link',
        'data-cms-file-link': '',
      }),
      0,
    ]
  },
})
