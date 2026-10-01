export const RICH_TEXT_ATTACHMENT_MIME = 'application/x-richtext-attachment'

export interface RichTextAttachmentPayload {
  id: number
  url?: string
  publicUrl?: string
  fullPath?: string
  mimeType?: string
  filename?: string
  originalFilename?: string
  thumbnailUrl?: string
}
