import { afterEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, reactive, type App } from 'vue'
import DataGridTable from './DataGridTable.vue'
import type { Config, DataItem } from '../../composables/useDataGrid'
import { isRichTextJSON, renderRichTextPreview } from './richTextPreview'

const documentJSON = (content: unknown) => JSON.stringify({ type: 'doc', content })
const textNode = (text: unknown, marks?: unknown) => ({ type: 'text', text, marks })
const preview = (content: unknown) => {
  const element = document.createElement('div')
  element.innerHTML = renderRichTextPreview(documentJSON(content))
  return element
}
const linkPreview = (href: unknown) =>
  preview([
    {
      type: 'paragraph',
      content: [textNode('Harmless link label', [{ type: 'link', attrs: { href } }])],
    },
  ])

// Payload tests never activate links, execute scripts or introduce network loads.
describe('DataGrid richtext safety: escaped text, fixed tags and URL allowlist', () => {
  it.each([
    '<b data-safety-marker="text">literal text</b>',
    '</p><span data-safety-marker="breakout">literal text</span>',
    '&lt;b data-safety-marker=&quot;entity&quot;&gt;literal&lt;/b&gt;',
    `Text with <, >, &, " and '`,
  ])('preserves text literally through nested supported marks: %s', (value) => {
    const element = preview([
      {
        type: 'paragraph',
        content: [textNode(value, [{ type: 'bold' }, { type: 'italic' }, { type: 'code' }])],
      },
    ])
    expect(element.textContent).toBe(value)
    expect(element.querySelector('[data-safety-marker]')).toBeNull()
    expect(element.querySelector('p > code > em > strong')?.textContent).toBe(value)
  })

  it.each([
    'https://example.test/path?q=one&next=two#fragment',
    'HTTP://example.test/path',
    '/admin/records/42',
    './records/42',
    '../records/42',
    'records/42',
    '?page=2',
    '#details',
    'mailto:person@example.test?subject=Hello',
    'tel:+1-555-0100',
  ])('keeps an allowed link: %s', (href) => {
    const element = linkPreview(href)
    expect(element.querySelector('a')?.getAttribute('href')).toBe(href)
    expect(element.textContent).toBe('Harmless link label')
  })

  it.each([
    'javascript:void(0)',
    'JaVaScRiPt:void(0)',
    ' javascript:void(0) ',
    'java\tscript:void(0)',
    'java\nscript:void(0)',
    'java\rscript:void(0)',
    '\u0000javascript:void(0)',
    'java script:void(0)',
    'javascript\u2028:void(0)',
    'data:text/plain,harmless',
    'vbscript:harmless',
    'file:///harmless',
    'ftp://example.test/harmless',
    'blob:https://example.test/harmless',
    'about:blank',
    '//example.test/ambiguous',
    '\\example.test/ambiguous',
    '/\\example.test/ambiguous',
    '/path\u007f',
    '/path\u0085',
    'https:example.test',
    'https:/example.test',
    'https:///example.test',
    'https://',
    'https://[invalid',
    'javascript%3avoid(0)',
    'mailto:',
    'mailto://person@example.test',
    'tel:',
    'tel://5550100',
    '',
    null,
    42,
    { href: '/valid' },
  ])('drops disallowed or malformed link %j while retaining its label', (href) => {
    const element = linkPreview(href)
    expect(element.querySelector('a')).toBeNull()
    expect(element.textContent).toBe('Harmless link label')
  })

  it.each([
    '#" data-safety-marker="attribute',
    '/search?q=" data-safety-marker="attribute&literal=<tag>',
    'https://example.test/search?q=" data-safety-marker="attribute',
    '#&quot; data-safety-marker=&quot;entity',
  ])('escapes every accepted attribute without interpreting quotes or entities: %s', (href) => {
    const element = linkPreview(href)
    const link = element.querySelector('a')!
    expect(link.getAttribute('href')).toBe(href)
    expect(Array.from(link.attributes).map((attribute) => attribute.name)).toEqual(['href'])
    expect(element.querySelector('[data-safety-marker]')).toBeNull()
  })

  it.each([1, 2, 3, 4, 5, 6])('retains supported heading level %i', (level) => {
    const element = preview([{ type: 'heading', attrs: { level }, content: [textNode('Heading')] }])
    expect(element.querySelector(`h${level}`)?.textContent).toBe('Heading')
  })

  it.each([0, -1, 7, 100, 1.5, '2', null, true, {}, [], '2 data-safety-marker="heading"'])(
    'bounds malformed heading level %j to a fixed h1 tag',
    (level) => {
      const element = preview([
        { type: 'heading', attrs: { level }, content: [textNode('Heading')] },
      ])
      expect(element.innerHTML).toBe('<h1>Heading</h1>')
    },
  )

  it('keeps the established harmless block and mark formatting', () => {
    const content = [
      { type: 'bulletList', content: [{ type: 'listItem', content: [textNode('Bullet')] }] },
      { type: 'orderedList', content: [{ type: 'listItem', content: [textNode('Numbered')] }] },
      { type: 'blockquote', content: [{ type: 'paragraph', content: [textNode('Quote')] }] },
      { type: 'codeBlock', content: [textNode('<code literal>')] },
      {
        type: 'paragraph',
        content: [
          textNode('Underline', [{ type: 'underline' }]),
          { type: 'hardBreak' },
          textNode('Strike', [{ type: 'strike' }]),
        ],
      },
      { type: 'image', attrs: { src: 'https://example.invalid/never-requested.png' } },
    ]
    const element = preview(content)
    expect(element.querySelector('ul > li')?.textContent).toBe('Bullet')
    expect(element.querySelector('ol > li')?.textContent).toBe('Numbered')
    expect(element.querySelector('blockquote > p')?.textContent).toBe('Quote')
    expect(element.querySelector('pre > code')?.textContent).toBe('<code literal>')
    expect(element.querySelector('u')?.textContent).toBe('Underline')
    expect(element.querySelector('s')?.textContent).toBe('Strike')
    expect(element.querySelector('br')).not.toBeNull()
    expect(element.querySelector('img')).toBeNull()
  })

  it.each([null, 42, 'not-an-array', {}, { map: 'not-a-function' }])(
    'handles malformed doc content %j without rendering the input as HTML',
    (content) => {
      expect(preview(content).innerHTML).toBe('')
    },
  )

  it('ignores malformed node/marks/attrs shapes and preserves safe siblings', () => {
    const element = preview([
      null,
      42,
      [],
      '<span data-safety-marker="node">ignored</span>',
      { type: 'paragraph', content: { map: '<span data-safety-marker="content">ignored</span>' } },
      { type: 'heading', attrs: [], content: [textNode('Heading')] },
      textNode('Ignored malformed marks', { type: 'bold' }),
      textNode('Safe siblings', [null, [], 42, { type: 'link', attrs: null }, { type: 'bold' }]),
      textNode({ toString: '<span data-safety-marker="text">ignored</span>' }),
    ])
    expect(element.querySelector('[data-safety-marker]')).toBeNull()
    expect(element.querySelector('h1')?.textContent).toBe('Heading')
    expect(element.querySelector('strong')?.textContent).toBe('Safe siblings')
    expect(element.textContent).toContain('Ignored malformed marks')
  })

  it.each([
    '<span data-safety-marker="fallback">not JSON</span>',
    '{"type":"doc",',
    JSON.stringify({
      type: 'paragraph',
      text: '<span data-safety-marker="fallback">not doc</span>',
    }),
  ])(
    'escapes malformed/non-doc fallback even when the renderer is called directly: %s',
    (value) => {
      const element = document.createElement('div')
      element.innerHTML = renderRichTextPreview(value)
      expect(element.textContent).toBe(value)
      expect(element.children).toHaveLength(0)
      expect(isRichTextJSON(value)).toBe(false)
    },
  )
})

const apps = new Set<App>()
const baseConfig: Config = {
  columns: [
    { key: 'name', label: 'Name', title: 'Name', sortable: true },
    { key: 'zero', label: 'Count', title: 'Count', type: 'number' },
  ],
  behaviour: { selectable: true },
  ui: { idKey: 'uuid' },
}
const row = (uuid: string | number): DataItem => ({
  item: { uuid, name: `Row ${uuid}`, zero: 0 },
  actions: [],
})
type TableProps = Pick<
  InstanceType<typeof DataGridTable>['$props'],
  'config' | 'items' | 'sortBy' | 'sortOrder' | 'selectedItems' | 'allSelected'
>
type TableFixtureProps = { -readonly [Key in keyof TableProps]: TableProps[Key] } & {
  onSort?: (columnKey: string) => void
  onToggleSelectAll?: () => void
  onToggleSelectItem?: (id: string | number) => void
}
const mountTable = (overrides: Partial<TableFixtureProps> = {}) => {
  const props = reactive<TableFixtureProps>({
    config: baseConfig,
    items: [row(0), row('b')],
    sortBy: '',
    sortOrder: 'asc',
    selectedItems: [],
    allSelected: false,
    ...overrides,
  })
  const element = document.createElement('div')
  document.body.append(element)
  const app = createApp(defineComponent({ setup: () => () => h(DataGridTable, props) }))
  apps.add(app)
  app.mount(element)
  return {
    element,
    props,
    unmount: () => {
      app.unmount()
      apps.delete(app)
      element.remove()
    },
  }
}
const checkboxes = (element: ParentNode) =>
  Array.from(element.querySelectorAll<HTMLButtonElement>('button[role="checkbox"]'))
afterEach(() => {
  apps.forEach((app) => app.unmount())
  apps.clear()
  document.body.innerHTML = ''
})

describe('DataGridTable actual primitives, native semantics and lifecycle', () => {
  it('emits one row/select-all callback with custom IDs including zero and reflects controlled updates', async () => {
    const onToggleSelectAll = vi.fn()
    const onToggleSelectItem = vi.fn()
    const { element, props } = mountTable({ onToggleSelectAll, onToggleSelectItem })
    const controls = checkboxes(element)
    expect(controls).toHaveLength(3)
    expect(controls.map((control) => control.getAttribute('aria-checked'))).toEqual([
      'false',
      'false',
      'false',
    ])
    expect(controls.map((control) => control.getAttribute('aria-label'))).toEqual([
      'Выбрать все строки',
      'Выбрать строку 0',
      'Выбрать строку b',
    ])
    controls[1].click()
    controls[2].click()
    controls[0].click()
    expect(onToggleSelectItem.mock.calls).toEqual([[0], ['b']])
    expect(onToggleSelectAll.mock.calls).toEqual([[]])
    props.selectedItems = [0, 'b']
    props.allSelected = true
    await nextTick()
    expect(controls.map((control) => control.getAttribute('aria-checked'))).toEqual([
      'true',
      'true',
      'true',
    ])
    controls[0].click()
    props.selectedItems = []
    props.allSelected = false
    await nextTick()
    expect(onToggleSelectAll.mock.calls).toEqual([[], []])
    expect(controls.map((control) => control.getAttribute('aria-checked'))).toEqual([
      'false',
      'false',
      'false',
    ])
  })

  it('renders focusable type=button sort controls and only marks the active header aria-sort', async () => {
    const onSort = vi.fn()
    const { element, props } = mountTable({ onSort })
    const heads = element.querySelectorAll('th')
    const sortButton = heads[1].querySelector<HTMLButtonElement>('button')!
    expect(sortButton.type).toBe('button')
    expect(sortButton.tabIndex).toBe(0)
    expect(sortButton.textContent?.trim()).toBe('Name')
    expect(heads[1].getAttribute('scope')).toBe('col')
    expect(heads[2].querySelector('button')).toBeNull()
    expect(element.querySelector('[aria-sort]')).toBeNull()
    sortButton.focus()
    expect(document.activeElement).toBe(sortButton)
    sortButton.click()
    heads[2].click()
    expect(onSort.mock.calls).toEqual([['name']])
    props.sortBy = 'name'
    await nextTick()
    expect(heads[1].getAttribute('aria-sort')).toBe('ascending')
    props.sortOrder = 'desc'
    await nextTick()
    expect(heads[1].getAttribute('aria-sort')).toBe('descending')
    props.sortBy = 'zero'
    await nextTick()
    expect(element.querySelector('[aria-sort]')).toBeNull()
  })

  it('preserves Reka checkbox Enter behavior without a duplicate custom keyboard handler', () => {
    const onToggleSelectItem = vi.fn()
    const { element } = mountTable({ onToggleSelectItem })
    const control = checkboxes(element)[1]
    control.focus()
    expect(document.activeElement).toBe(control)
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true })
    control.dispatchEvent(event)
    expect(event.defaultPrevented).toBe(true)
    expect(onToggleSelectItem).not.toHaveBeenCalled()
    // JSDOM does not implement native Space/Enter button activation. Its real
    // browser verification remains separate; calling .click() is not that proof.
  })

  it('retains checkbox and row identity across reorder and updates with stable custom IDs', async () => {
    const onToggleSelectItem = vi.fn()
    const { element, props } = mountTable({ onToggleSelectItem, selectedItems: [0] })
    const before = Array.from(element.querySelectorAll('tbody tr'))
    const zeroControl = checkboxes(element)[1]
    props.items = [row('b'), { ...row(0), item: { uuid: 0, name: 'Updated zero', zero: 0 } }]
    await nextTick()
    const after = Array.from(element.querySelectorAll('tbody tr'))
    expect(after).toEqual([before[1], before[0]])
    expect(after[1].querySelector('[role="checkbox"]')).toBe(zeroControl)
    expect(zeroControl.getAttribute('aria-checked')).toBe('true')
    expect(after[1].textContent).toContain('Updated zero')
    zeroControl.click()
    expect(onToggleSelectItem.mock.calls).toEqual([[0]])
  })

  it('keeps empty action cells aligned as actions appear and disappear', async () => {
    const { element, props } = mountTable()
    const before = Array.from(element.querySelectorAll('tbody tr'))
    props.items = [{ ...row(0), actions: [{ key: 'edit', label: 'Edit' }] }, row('b')]
    await nextTick()
    expect(element.querySelectorAll('th')).toHaveLength(4)
    expect(before.map((entry) => entry.querySelectorAll('td').length)).toEqual([4, 4])
    expect(before[1].querySelector('td:last-child')?.textContent).toBe('')
    props.items = [row(0), row('b')]
    await nextTick()
    expect(element.querySelectorAll('th')).toHaveLength(3)
    expect(before.map((entry) => entry.querySelectorAll('td').length)).toEqual([3, 3])
    expect(Array.from(element.querySelectorAll('tbody tr'))).toEqual(before)
  })

  it('mounts, interacts and unmounts selectable tables repeatedly without retained controls', async () => {
    for (let run = 0; run < 3; run++) {
      const onToggleSelectItem = vi.fn()
      const { element, unmount } = mountTable({
        items: Array.from({ length: 10 }, (_, index) => row(index)),
        onToggleSelectItem,
      })
      const control = checkboxes(element)[1]
      control.click()
      await nextTick()
      expect(onToggleSelectItem.mock.calls).toEqual([[0]])
      unmount()
      await nextTick()
      expect(control.isConnected).toBe(false)
      expect(document.querySelector('[role="checkbox"]')).toBeNull()
    }
  })
})
