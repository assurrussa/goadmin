export type RichTextValueVersion = string | number | null

export type RichTextFeature =
  | 'bold'
  | 'italic'
  | 'underline'
  | 'strike'
  | 'heading'
  | 'bulletList'
  | 'orderedList'
  | 'taskList'
  | 'image'
  | 'video'
  | 'fileLink'
  | 'link'
  | 'table'
  | 'blockquote'
  | 'codeBlock'
  | 'horizontalRule'
  | 'rawJson'

export type RichTextInvalidContentSource = 'initial' | 'external' | 'raw-json'

export interface RichTextInvalidContent {
  error: Error
  source: RichTextInvalidContentSource
  value: unknown
  valueVersion: RichTextValueVersion
}

export interface RichTextJSONNode {
  type: string
  attrs?: Record<string, unknown>
  content?: RichTextJSONNode[]
  marks?: Array<{ type: string; attrs?: Record<string, unknown> }>
  text?: string
}

export interface RichTextCanonicalMediaSelection {
  node: RichTextJSONNode
  previewUrl?: string
}

export interface RichTextLegacyMediaSelection {
  id: string | number
  url: string
  filename?: string
  mimeType?: string
}

export type RichTextMediaSelection = RichTextCanonicalMediaSelection | RichTextLegacyMediaSelection

export interface RichTextMediaPickerRequest {
  kind: 'image' | 'video' | 'file'
  allowedNodes: readonly string[] | null
  entityId: number | string | null
  entityType: string
  valueVersion: RichTextValueVersion
}

export type RichTextMediaPicker = (
  request: RichTextMediaPickerRequest,
) => RichTextMediaSelection | null | Promise<RichTextMediaSelection | null>

const featureRequirements: Partial<
  Record<RichTextFeature, { nodes?: string[]; marks?: string[] }>
> = {
  bold: { marks: ['bold'] },
  italic: { marks: ['italic'] },
  underline: { marks: ['underline'] },
  strike: { marks: ['strike'] },
  heading: { nodes: ['heading'] },
  bulletList: { nodes: ['bulletList'] },
  orderedList: { nodes: ['orderedList'] },
  taskList: { nodes: ['taskList'] },
  image: { nodes: ['cmsImage', 'image'] },
  video: { nodes: ['cmsVideo', 'video'] },
  fileLink: { marks: ['cmsFileLink'] },
  link: { marks: ['link'] },
  table: { nodes: ['table'] },
  blockquote: { nodes: ['blockquote'] },
  codeBlock: { nodes: ['codeBlock'] },
  horizontalRule: { nodes: ['horizontalRule'] },
}

const normalizeFeature = (feature: string): string => feature.replaceAll('-', '').toLowerCase()

export const isRichTextFeatureAllowed = (
  feature: RichTextFeature,
  allowedFeatures?: readonly string[] | null,
  allowedNodes?: readonly string[] | null,
  allowedMarks?: readonly string[] | null,
): boolean => {
  if (
    allowedFeatures &&
    !allowedFeatures.some((candidate) => normalizeFeature(candidate) === normalizeFeature(feature))
  ) {
    return false
  }

  const requirement = featureRequirements[feature]
  if (
    allowedNodes &&
    requirement?.nodes &&
    !requirement.nodes.some((node) => allowedNodes.includes(node))
  ) {
    return false
  }
  if (
    allowedMarks &&
    requirement?.marks &&
    !requirement.marks.some((mark) => allowedMarks.includes(mark))
  ) {
    return false
  }
  return true
}

export const isCanonicalMediaSelection = (
  selection: RichTextMediaSelection,
): selection is RichTextCanonicalMediaSelection => 'node' in selection
