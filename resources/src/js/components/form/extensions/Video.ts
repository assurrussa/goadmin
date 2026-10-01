import { Node, mergeAttributes } from '@tiptap/core'

export interface VideoAttributes {
  src: string
  title?: string | null
  controls?: boolean
  poster?: string | null
  'data-file-id'?: number | string | null
  'data-type'?: string
}

declare module '@tiptap/core' {
  interface Commands<ReturnType> {
    video: {
      /**
       * Insert a video node
       */
      setVideo: (attributes: VideoAttributes) => ReturnType
    }
  }
}

export const VideoExtension = Node.create({
  name: 'video',
  group: 'block',
  inline: false,
  atom: true,
  draggable: true,

  addOptions() {
    return {
      HTMLAttributes: {
        class: 'editor-video',
        controls: true,
      } as Record<string, unknown>,
    }
  },

  addAttributes() {
    return {
      src: {
        default: null,
      },
      title: {
        default: null,
      },
      controls: {
        default: true,
        parseHTML: (element) => element.hasAttribute('controls'),
      },
      poster: {
        default: null,
      },
      'data-file-id': {
        default: null,
        parseHTML: (element) => {
          const raw = element.getAttribute('data-file-id')
          if (raw == null) return null
          const parsed = Number(raw)
          return Number.isFinite(parsed) ? parsed : raw
        },
      },
      'data-type': {
        default: 'video',
      },
    }
  },

  parseHTML() {
    return [
      {
        tag: 'video[src]',
      },
    ]
  },

  renderHTML({ HTMLAttributes }) {
    const attrs = mergeAttributes(this.options.HTMLAttributes, HTMLAttributes)
    return ['div', { class: 'editor-video-wrapper' }, ['video', attrs]]
  },

  addCommands() {
    return {
      setVideo:
        (attributes: VideoAttributes) =>
        ({ commands }) => {
          if (!attributes?.src) {
            return false
          }

          return commands.insertContent({
            type: this.name,
            attrs: {
              ...attributes,
              controls: attributes.controls ?? true,
              'data-type': attributes['data-type'] ?? 'video',
            },
          })
        },
    }
  },
})

export default VideoExtension
