/**
 * Does this text promise something that should be in the message?
 *
 * The check is deliberately about intent, not about files: it runs only when
 * nothing is attached, and its whole job is to make the writer look once more.
 * A false positive costs one click; a false negative sends a message without
 * the thing it was about.
 *
 * Turkish is included because the people this client is for write Turkish even
 * though the interface is English. `\b` keeps "ek" from matching "ekmek" and
 * "ekran".
 */
const ATTACHMENT_INTENT =
  /\b(?:attach\w*|enclos\w*|ek(?:te|li|ler|i|in|le|len\w*|ley\w*)?|ilişik\w*)\b/

export function mentionsAttachment(text: string): boolean {
  return ATTACHMENT_INTENT.test(text.toLocaleLowerCase('tr'))
}

/**
 * The part of the body the writer actually wrote.
 *
 * A reply carries the original below the cursor, and an original that talks
 * about its own attachment must not make every answer ask the question again.
 * When the quote cannot be found the whole body is used: a reminder that did
 * not appear is worse than one that appeared needlessly.
 */
export function textBeforeQuote(body: string, quoted?: string): string {
  if (!quoted) return body
  const at = body.indexOf(quoted)
  return at >= 0 ? body.slice(0, at) : body
}
