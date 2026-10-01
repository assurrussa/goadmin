import { describe, expect, it } from 'vitest'
import { isCanonicalMediaSelection, isRichTextFeatureAllowed } from './richTextContract'

describe('richTextContract', () => {
  it('keeps the legacy toolbar enabled when no policy is supplied', () => {
    expect(isRichTextFeatureAllowed('image')).toBe(true)
    expect(isRichTextFeatureAllowed('taskList')).toBe(true)
  })

  it('maps canonical CMS nodes to image and video features', () => {
    const nodes = ['doc', 'paragraph', 'text', 'cmsImage', 'cmsVideo', 'table']
    const marks = ['cmsFileLink']

    expect(isRichTextFeatureAllowed('image', null, nodes)).toBe(true)
    expect(isRichTextFeatureAllowed('video', null, nodes)).toBe(true)
    expect(isRichTextFeatureAllowed('fileLink', null, nodes, marks)).toBe(true)
    expect(isRichTextFeatureAllowed('table', null, nodes, marks)).toBe(true)
    expect(isRichTextFeatureAllowed('taskList', null, nodes)).toBe(false)
  })

  it('intersects feature, node, and mark allowlists', () => {
    expect(isRichTextFeatureAllowed('bold', ['bold'], null, ['bold'])).toBe(true)
    expect(isRichTextFeatureAllowed('bold', ['italic'], null, ['bold'])).toBe(false)
    expect(isRichTextFeatureAllowed('bold', ['bold'], null, ['italic'])).toBe(false)
    expect(isRichTextFeatureAllowed('rawJson', ['raw-json'])).toBe(true)
  })

  it('distinguishes canonical picker results from legacy URL results', () => {
    expect(
      isCanonicalMediaSelection({
        node: { type: 'cmsImage', attrs: { id: 'image-1', assetId: 'asset-1' } },
      }),
    ).toBe(true)
    expect(isCanonicalMediaSelection({ id: 1, url: '/files/1' })).toBe(false)
  })
})
