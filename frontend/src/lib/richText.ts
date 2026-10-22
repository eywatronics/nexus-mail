/**
 * The HTML the composer is allowed to send.
 *
 * Contenteditable is a browser surface: pasted markup can carry scripts,
 * event handlers and remote images that report when the message is opened.
 * The editor intercepts paste, but `innerHTML` has more than one way in, so
 * the value that leaves this module is the only one that is trusted.
 *
 * The allowlist is deliberately small. Every tag here is one the toolbar can
 * produce or a mail reader universally renders; anything else is unwrapped to
 * its text rather than dropped, so a paste loses styling but not words.
 */

const ALLOWED = new Set([
  'P',
  'DIV',
  'BR',
  'B',
  'STRONG',
  'I',
  'EM',
  'U',
  'A',
  'UL',
  'OL',
  'LI',
  'BLOCKQUOTE',
])

// Never unwrapped: their text is not content, and their children can execute.
const DISCARD = new Set(['SCRIPT', 'STYLE', 'IFRAME', 'OBJECT', 'EMBED', 'IMG', 'LINK', 'META'])

const BLOCKS = new Set(['P', 'DIV', 'LI', 'BLOCKQUOTE', 'UL', 'OL', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6'])

export function safeLinkHref(href: string): string | null {
  const value = href.trim()
  return /^(https?:|mailto:)/i.test(value) ? value : null
}

/**
 * A person typing "example.com" means a web address; one typing a bare
 * mailbox means an address. Anything already carrying a scheme goes through
 * the same allowlist as a stored link.
 */
export function normalizeLinkHref(input: string): string | null {
  const value = input.trim()
  if (value === '') return null
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) return safeLinkHref(value)
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) return `mailto:${value}`
  return `https://${value}`
}

export function sanitizeComposeHTML(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  clean(doc.body)
  return doc.body.innerHTML
}

function clean(parent: Node) {
  for (const child of Array.from(parent.childNodes)) {
    if (child.nodeType === Node.TEXT_NODE) continue
    if (child.nodeType !== Node.ELEMENT_NODE) {
      child.remove()
      continue
    }

    const element = child as HTMLElement
    if (DISCARD.has(element.tagName)) {
      element.remove()
      continue
    }

    clean(element)

    if (!ALLOWED.has(element.tagName)) {
      element.replaceWith(...Array.from(element.childNodes))
      continue
    }

    for (const attribute of Array.from(element.attributes)) {
      if (element.tagName === 'A' && attribute.name === 'href') {
        const href = safeLinkHref(attribute.value)
        if (href === null) element.removeAttribute('href')
        else element.setAttribute('href', href)
      } else {
        element.removeAttribute(attribute.name)
      }
    }

    if (element.tagName === 'A' && !element.hasAttribute('href')) {
      element.replaceWith(...Array.from(element.childNodes))
    }
  }
}

/**
 * The plain-text alternative of an HTML body.
 *
 * Block elements end a line; `<br>` is a line. Used for the text part of a
 * rich message and for switching to plain-text mode, so both readers and the
 * writer see the same words.
 */
export function htmlToText(html: string): string {
  const doc = new DOMParser().parseFromString(html, 'text/html')
  const out = textOf(doc.body)
    .replace(/\u00a0/g, ' ')
    .replace(/[ \t]+\n/g, '\n')
    .replace(/\n{3,}/g, '\n\n')
  return out.replace(/\n+$/, '')
}

function textOf(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? ''
  if (node.nodeType !== Node.ELEMENT_NODE) return ''

  const element = node as HTMLElement
  if (element.tagName === 'BR') return '\n'

  const inner = Array.from(element.childNodes).map(textOf).join('')
  if (element.tagName === 'LI') {
    return inner.endsWith('\n') ? inner : inner + '\n'
  }
  // A contenteditable in Chromium makes a new line a div; that is one line,
  // not a paragraph break. Lists get the blank line a block gets.
  if (element.tagName === 'DIV') {
    return inner.endsWith('\n') ? inner : inner + '\n'
  }
  if (element.tagName === 'UL' || element.tagName === 'OL') {
    return inner.endsWith('\n\n') ? inner : inner + '\n\n'
  }
  if (BLOCKS.has(element.tagName)) {
    return inner.endsWith('\n\n') ? inner : inner + '\n\n'
  }
  return inner
}

export function escapeHTML(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

/** Plain text as paragraphs, with line breaks kept inside each one. */
export function textToHTML(text: string): string {
  if (text === '') return ''
  return text
    .split(/\n{2,}/)
    .map((block) => `<p>${escapeHTML(block).replace(/\n/g, '<br>')}</p>`)
    .join('')
}
