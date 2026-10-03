// Only these fixed tags are generated. All content and attributes originating
// from the document must be escaped before entering DataGridTable's v-html sink.
const htmlEntities: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}
const escapeHtml = (value: string): string =>
  value.replace(/[&<>"']/g, (character) => htmlEntities[character])

const isRecord = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === 'object' && !Array.isArray(value)

// Relative paths/fragments and explicit http(s), mailto and tel links are useful
// in previews. Reject browser-normalized control characters, backslashes,
// scheme-relative references and unsupported/ambiguous schemes. Escaping the
// returned URL is still required: a URL allowlist is not HTML sanitization.
const safeHref = (value: unknown): string | null => {
  if (typeof value !== 'string' || /[\u0000-\u001f\u007f-\u009f\\]/u.test(value)) return null
  const href = value.trim()
  if (!href || href.startsWith('//')) return null

  const scheme = /^([a-z][a-z\d+.-]*):/i.exec(href)?.[1].toLowerCase()
  if (scheme) {
    if (scheme === 'http' || scheme === 'https') {
      if (!/^https?:\/\/[^/]/i.test(href)) return null
      try {
        const url = new URL(href)
        return url.hostname ? href : null
      } catch {
        return null
      }
    }
    if (scheme === 'mailto' || scheme === 'tel') {
      const destination = href.slice(scheme.length + 1)
      return destination && !destination.startsWith('//') && !/\s/u.test(destination) ? href : null
    }
    return null
  }

  // A colon in the first path segment is a malformed/unsupported scheme, not
  // an ordinary relative path. Encoded colons there are rejected conservatively.
  const firstSegment = href.split(/[/?#]/, 1)[0]
  return /:|%3a/i.test(firstSegment) ? null : href
}

export const isRichTextJSON = (value: unknown): boolean => {
  if (typeof value !== 'string') return false
  try {
    const parsed: unknown = JSON.parse(value)
    return isRecord(parsed) && parsed.type === 'doc'
  } catch {
    return false
  }
}

const renderContent = (value: unknown): string =>
  Array.isArray(value) ? value.map(renderNode).join('') : ''

const renderNode = (value: unknown): string => {
  if (!isRecord(value)) return ''

  switch (value.type) {
    case 'paragraph':
      return `<p>${renderContent(value.content)}</p>`
    case 'heading': {
      const candidate = isRecord(value.attrs) ? value.attrs.level : undefined
      const level =
        typeof candidate === 'number' &&
        Number.isInteger(candidate) &&
        candidate >= 1 &&
        candidate <= 6
          ? candidate
          : 1
      return `<h${level}>${renderContent(value.content)}</h${level}>`
    }
    case 'text': {
      let html = typeof value.text === 'string' ? escapeHtml(value.text) : ''
      if (!Array.isArray(value.marks)) return html
      for (const mark of value.marks) {
        if (!isRecord(mark)) continue
        switch (mark.type) {
          case 'bold':
            html = `<strong>${html}</strong>`
            break
          case 'italic':
            html = `<em>${html}</em>`
            break
          case 'underline':
            html = `<u>${html}</u>`
            break
          case 'strike':
            html = `<s>${html}</s>`
            break
          case 'code':
            html = `<code>${html}</code>`
            break
          case 'link': {
            const href = safeHref(isRecord(mark.attrs) ? mark.attrs.href : undefined)
            if (href !== null) html = `<a href="${escapeHtml(href)}">${html}</a>`
            break
          }
        }
      }
      return html
    }
    case 'bulletList':
      return `<ul>${renderContent(value.content)}</ul>`
    case 'orderedList':
      return `<ol>${renderContent(value.content)}</ol>`
    case 'listItem':
      return `<li>${renderContent(value.content)}</li>`
    case 'blockquote':
      return `<blockquote>${renderContent(value.content)}</blockquote>`
    case 'codeBlock':
      return `<pre><code>${renderContent(value.content)}</code></pre>`
    case 'hardBreak':
      return '<br>'
    default:
      return renderContent(value.content)
  }
}

export const renderRichTextPreview = (value: unknown): string => {
  if (typeof value !== 'string') return ''
  try {
    const parsed: unknown = JSON.parse(value)
    return isRecord(parsed) && parsed.type === 'doc' ? renderNode(parsed) : escapeHtml(value)
  } catch {
    // Even malformed documents must never fall back to raw HTML at this sink.
    return escapeHtml(value)
  }
}
