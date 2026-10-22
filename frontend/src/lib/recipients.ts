/**
 * The small amount of address-line handling the composer does for display.
 *
 * This is not address parsing. The backend parses, once, when the message is
 * built; these helpers only split what was typed at the separators a person
 * uses, so the field can show committed addresses as pills and keep the rest
 * in the input. Anything ambiguous stays as typed and is handed over intact.
 */

/**
 * Splits on commas and semicolons that are not inside a quoted display name or
 * angle brackets. A trailing separator produces a trailing empty entry, which
 * is what tells the caller the address before it was completed.
 */
export function splitRecipientLine(line: string): string[] {
  const parts: string[] = []
  let current = ''
  let quoted = false
  let angles = 0

  for (let i = 0; i < line.length; i++) {
    const ch = line[i]

    if (quoted) {
      current += ch
      if (ch === '"' && line[i - 1] !== '\\') quoted = false
      continue
    }
    if (ch === '"') {
      quoted = true
      current += ch
      continue
    }
    if (ch === '<') angles++
    if (ch === '>' && angles > 0) angles--

    if ((ch === ',' || ch === ';') && angles === 0) {
      parts.push(current.trim())
      current = ''
      continue
    }
    current += ch
  }

  parts.push(current.trim())
  return parts
}

/** The address part of a pill or a hand-typed entry. */
export function addressIn(token: string): string {
  const open = token.lastIndexOf('<')
  const close = token.lastIndexOf('>')
  if (open >= 0 && close > open) return token.slice(open + 1, close).trim()
  return token.trim()
}

/**
 * What a pill shows.
 *
 * The display name when there is one, because that is what the writer typed
 * or chose; the full address is in the pill's title. A row of full
 * "Name <address>" strings leaves no room for the next address at this width.
 */
export function pillLabel(token: string): string {
  const open = token.lastIndexOf('<')
  if (open < 0) return token
  let name = token.slice(0, open).trim()
  if (name.startsWith('"') && name.endsWith('"')) {
    name = name.slice(1, -1).replace(/\\"/g, '"')
  }
  if (name !== '') return name
  return addressIn(token)
}

/**
 * A suggestion as it should appear in the address line.
 *
 * Display names containing a comma, quote, angle bracket or semicolon are
 * quoted. Without that a name like "Kabatepe, İsmet" would be two addresses
 * by the time the backend parsed the line.
 */
export function formatRecipient(name: string, addr: string): string {
  const display = name.trim()
  if (display === '') return addr
  const quoted = /[",<>;]/.test(display)
  const safe = quoted ? `"${display.replace(/"/g, '\\"')}"` : display
  return `${safe} <${addr}>`
}

/** The line the backend receives, in the one shape this field produces. */
export function formatRecipientLine(pills: string[], draft: string): string {
  const parts = [...pills]
  const rest = draft.trim()
  if (rest !== '') parts.push(rest)
  return parts.join(', ')
}
