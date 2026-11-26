const allowedTags = new Set([
  'a',
  'blockquote',
  'br',
  'caption',
  'code',
  'div',
  'em',
  'figcaption',
  'figure',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'hr',
  'img',
  'li',
  'ol',
  'p',
  'pre',
  'span',
  'strong',
  'table',
  'tbody',
  'td',
  'tfoot',
  'th',
  'thead',
  'tr',
  'u',
  'ul',
])

const blockedTags = new Set(['embed', 'form', 'iframe', 'input', 'link', 'meta', 'noscript', 'object', 'script', 'style', 'svg', 'textarea'])
const voidTags = new Set(['br', 'hr', 'img'])

function isAllowedImageURL(value: string) {
  const url = value.trim()
  return url.startsWith('/uploads/') || /^https?:\/\/[^\s]+$/i.test(url)
}

function isAllowedLinkURL(value: string) {
  const url = value.trim()
  if ((url.startsWith('/') && !url.startsWith('//')) || url.startsWith('#')) {
    return true
  }
  return /^(https?:\/\/|mailto:|tel:)[^\s]+$/i.test(url)
}

function sanitizedAttribute(tag: string, name: string, value: string): [string, string] | null {
  const key = name.toLowerCase().trim()
  const cleanValue = value.trim()
  if (!key || key.startsWith('on')) {
    return null
  }
  if ((key === 'alt' || key === 'title') && (tag === 'img' || tag === 'a')) {
    return [key, cleanValue]
  }
  if ((key === 'colspan' || key === 'rowspan') && (tag === 'td' || tag === 'th')) {
    return [key, cleanValue]
  }
  if (key === 'src' && tag === 'img' && isAllowedImageURL(cleanValue)) {
    return [key, cleanValue]
  }
  if (key === 'href' && tag === 'a' && isAllowedLinkURL(cleanValue)) {
    return [key, cleanValue]
  }
  return null
}

function sanitizeWithDOMParser(raw: string) {
  const parser = new DOMParser()
  const documentNode = parser.parseFromString(`<body>${raw}</body>`, 'text/html')
  const output = documentNode.createElement('div')

  function appendCleanNode(parent: Node, node: Node) {
    if (node.nodeType === Node.TEXT_NODE) {
      parent.appendChild(documentNode.createTextNode(node.textContent || ''))
      return
    }
    if (node.nodeType !== Node.ELEMENT_NODE) {
      return
    }

    const source = node as HTMLElement
    const tag = source.tagName.toLowerCase()
    if (blockedTags.has(tag)) {
      return
    }
    if (!allowedTags.has(tag)) {
      Array.from(source.childNodes).forEach((child) => appendCleanNode(parent, child))
      return
    }

    const clean = documentNode.createElement(tag)
    Array.from(source.attributes).forEach((attribute) => {
      const sanitized = sanitizedAttribute(tag, attribute.name, attribute.value)
      if (sanitized) {
        clean.setAttribute(...sanitized)
      }
    })
    parent.appendChild(clean)
    Array.from(source.childNodes).forEach((child) => appendCleanNode(clean, child))
  }

  Array.from(documentNode.body.childNodes).forEach((node) => appendCleanNode(output, node))
  return output.innerHTML.trim()
}

function escapeAttribute(value: string) {
  return value.replaceAll('&', '&amp;').replaceAll('"', '&quot;').replaceAll('<', '&lt;').replaceAll('>', '&gt;')
}

function sanitizeWithoutDOM(raw: string) {
  let html = raw.replace(/<!--[\s\S]*?-->/g, '')
  html = html.replace(
    /<(script|style|iframe|object|embed|form|textarea|noscript|svg)\b[^>]*>[\s\S]*?<\/\1\s*>/gi,
    '',
  )
  html = html.replace(/<\/?(?:script|style|iframe|object|embed|form|input|link|meta|textarea|noscript|svg)\b[^>]*>/gi, '')

  return html
    .replace(/<\/?([a-zA-Z0-9-]+)([^>]*)>/g, (match, rawTag: string, rawAttributes: string) => {
      const tag = rawTag.toLowerCase()
      const closing = match.startsWith('</')
      if (!allowedTags.has(tag)) {
        return ''
      }
      if (closing) {
        return voidTags.has(tag) ? '' : `</${tag}>`
      }

      const attributes: string[] = []
      rawAttributes.replace(
        /([^\s=/>]+)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/g,
        (_attribute, name: string, doubleQuoted: string, singleQuoted: string, unquoted: string) => {
          const value = doubleQuoted ?? singleQuoted ?? unquoted ?? ''
          const sanitized = sanitizedAttribute(tag, name, value)
          if (sanitized) {
            attributes.push(`${sanitized[0]}="${escapeAttribute(sanitized[1])}"`)
          }
          return ''
        },
      )
      return `<${tag}${attributes.length ? ` ${attributes.join(' ')}` : ''}>`
    })
    .trim()
}

export function sanitizeProductDetailHtml(raw?: string | null) {
  const html = raw?.trim() || ''
  if (!html) {
    return ''
  }
  return typeof DOMParser === 'undefined' || typeof Node === 'undefined' ? sanitizeWithoutDOM(html) : sanitizeWithDOMParser(html)
}
