// Rich Text Parser Composable
// Конвертирует данные из различных форматов в TipTap совместимый формат

interface LegacyNode {
  type: string
  format?: string
  children?: LegacyNode[]
  text?: string
  bold?: boolean
  italic?: boolean
  underline?: boolean
  strikethrough?: boolean
  code?: boolean
}

/**
 * Composable для работы с Rich Text парсингом
 *
 * Пример использования:
 * ```typescript
 * import { useRichTextParser } from '@/composables/useRichTextParser'
 *
 * const richTextParser = useRichTextParser()
 * const parsedBody = richTextParser.parseBody(oldData.body || props.data.body)
 *  ```
 */
export const useRichTextParser = () => {
  /**
   * Парсит Rich Text данные из формы или базы данных
   * @param bodyData - данные для парсинга
   * @returns отпарсенные данные в TipTap формате
   */
  const parseBody = (bodyData: unknown) => {
    return parseRichTextBody(bodyData)
  }

  return {
    parseBody,
  }
}

/**
 * Основная функция парсинга Rich Text данных
 * Поддерживает различные форматы и автоматически конвертирует в TipTap формат
 */
const parseRichTextBody = (bodyData: unknown): unknown => {
  if (!bodyData) return null

  let parsedData = bodyData

  // Если это строка, пытаемся распарсить как JSON
  if (typeof bodyData === 'string') {
    try {
      parsedData = JSON.parse(bodyData)
    } catch (error) {
      console.warn('Rich Text Parser: Failed to parse body as JSON:', error)
      return bodyData
    }
  }

  // Если это массив (старый формат), конвертируем в TipTap формат
  if (Array.isArray(parsedData)) {
    console.log('Rich Text Parser: Converting old format to TipTap format')
    return convertToTipTapFormat(parsedData as LegacyNode[])
  }

  // Если это уже объект TipTap формата, возвращаем как есть
  if (
    typeof parsedData === 'object' &&
    parsedData !== null &&
    (parsedData as { type?: string }).type === 'doc'
  ) {
    return upgradeLegacyVideoNodes(parsedData)
  }

  // Если это обычный объект, но не TipTap формат, пытаемся конвертировать
  if (typeof parsedData === 'object' && parsedData !== null) {
    console.log('Rich Text Parser: Object detected, attempting conversion')
    return upgradeLegacyVideoNodes(parsedData)
  }

  return parsedData
}

/**
 * Конвертер в TipTap формат
 * Преобразует старый формат с children в новый формат с content
 */
const convertToTipTapFormat = (data: LegacyNode[]): Record<string, unknown> => {
  if (!Array.isArray(data)) return data as unknown as Record<string, unknown>

  const convertNode = (node: LegacyNode): Record<string, unknown> => {
    switch (node.type) {
      case 'paragraph':
        return {
          type: 'paragraph',
          content:
            node.children?.map((child) => ({
              type: 'text',
              text: child.text || '',
              marks: buildMarks(child),
            })) || [],
        }

      case 'list':
        return {
          type: node.format === 'unordered' ? 'bulletList' : 'orderedList',
          content:
            node.children?.map((item) => ({
              type: 'listItem',
              content: [
                {
                  type: 'paragraph',
                  content:
                    item.children?.map((child) => ({
                      type: 'text',
                      text: child.text || '',
                      marks: buildMarks(child),
                    })) || [],
                },
              ],
            })) || [],
        }

      default:
        // Неизвестные типы пропускаем как есть
        return node as unknown as Record<string, unknown>
    }
  }

  const buildMarks = (child: LegacyNode): { type: string }[] => {
    const marks: { type: string }[] = []
    if (child.bold) marks.push({ type: 'bold' })
    if (child.italic) marks.push({ type: 'italic' })
    if (child.underline) marks.push({ type: 'underline' })
    if (child.strikethrough) marks.push({ type: 'strike' })
    if (child.code) marks.push({ type: 'code' })
    return marks
  }

  return {
    type: 'doc',
    content: data.map(convertNode),
  }
}

const upgradeLegacyVideoNodes = (doc: unknown): unknown => {
  if (!doc || typeof doc !== 'object' || !Array.isArray((doc as { content?: unknown[] }).content)) {
    return doc
  }

  const parseVideoAttributes = (value: string) => {
    const tagMatch = value.match(/<video[^>]*>/i)
    if (!tagMatch) {
      return null
    }

    const attrs: Record<string, string> = {}
    const attrRegex = /([\w-:]+)\s*=\s*"([^"]*)"/g
    let attrMatch: RegExpExecArray | null

    while ((attrMatch = attrRegex.exec(tagMatch[0])) !== null) {
      attrs[attrMatch[1]] = attrMatch[2]
    }

    if (!attrs.src) {
      return null
    }

    const dataFileId = attrs['data-file-id']
    const parsedId = dataFileId ? Number(dataFileId) : null

    return {
      src: attrs.src,
      title: attrs.title ?? null,
      'data-type': attrs['data-type'] ?? 'video',
      'data-file-id': Number.isFinite(parsedId) ? parsedId : (dataFileId ?? null),
      controls: attrs.controls !== 'false',
      poster: attrs.poster ?? null,
    }
  }

  const transformContent = (nodes: unknown[]): unknown[] => {
    return nodes.flatMap((node) => {
      if (!node || typeof node !== 'object') {
        return node
      }

      const typedNode = node as {
        type?: string
        content?: unknown[]
        text?: string
        attrs?: Record<string, unknown>
      }

      if (
        typedNode.type === 'paragraph' &&
        Array.isArray(typedNode.content) &&
        typedNode.content.length === 1
      ) {
        const child = typedNode.content[0] as { type?: string; text?: string }
        if (
          child?.type === 'text' &&
          typeof child.text === 'string' &&
          child.text.includes('<video')
        ) {
          const attrs = parseVideoAttributes(child.text)
          if (attrs) {
            return [
              {
                type: 'video',
                attrs,
              },
            ]
          }
        }
      }

      if (Array.isArray(typedNode.content)) {
        return {
          ...typedNode,
          content: transformContent(typedNode.content),
        }
      }

      if (
        typedNode.type === 'text' &&
        typeof typedNode.text === 'string' &&
        typedNode.text.includes('<video')
      ) {
        const attrs = parseVideoAttributes(typedNode.text)
        if (attrs) {
          return [
            {
              type: 'video',
              attrs,
            },
          ]
        }
      }

      return node
    })
  }

  const typedDoc = doc as { content: unknown[] }
  return {
    ...typedDoc,
    content: transformContent(typedDoc.content),
  }
}
