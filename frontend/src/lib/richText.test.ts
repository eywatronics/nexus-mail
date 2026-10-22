import { describe, expect, it } from 'vitest'
import {
  escapeHTML,
  htmlToText,
  normalizeLinkHref,
  sanitizeComposeHTML,
  textToHTML,
} from './richText'

describe('sanitizeComposeHTML', () => {
  it('keeps the formatting the toolbar can produce', () => {
    const html = '<p><b>kalın</b> <i>eğik</i> <u>altı çizili</u></p><ul><li>bir</li></ul>'
    expect(sanitizeComposeHTML(html)).toBe(html)
  })

  it('unwraps tags it does not allow but keeps their words', () => {
    expect(sanitizeComposeHTML('<p><span style="color:red">metin</span></p>')).toBe(
      '<p>metin</p>',
    )
  })

  it('removes scripts and event handlers rather than sending them', () => {
    const clean = sanitizeComposeHTML(
      '<p onclick="steal()">metin</p><script>steal()</script>',
    )
    expect(clean).toBe('<p>metin</p>')
  })

  it('removes a javascript link but keeps its text', () => {
    expect(sanitizeComposeHTML('<p><a href="javascript:alert(1)">tıkla</a></p>')).toBe(
      '<p>tıkla</p>',
    )
  })

  it('keeps a safe link and strips its other attributes', () => {
    expect(
      sanitizeComposeHTML('<a href="https://example.com" target="_blank" onclick="x()">x</a>'),
    ).toBe('<a href="https://example.com">x</a>')
  })

  // A remote image in a message is a tracking pixel the moment it is opened.
  // Inline images get their own path later; a pasted <img> does not.
  it('drops images', () => {
    expect(sanitizeComposeHTML('<p>a<img src="https://track/x.png">b</p>')).toBe(
      '<p>ab</p>',
    )
  })
})

describe('htmlToText', () => {
  it('turns paragraphs into blank-line-separated text', () => {
    expect(htmlToText('<p>bir</p><p>iki</p>')).toBe('bir\n\niki')
  })

  it('keeps line breaks and list items', () => {
    expect(htmlToText('<div>bir<br>iki</div><ul><li>üç</li></ul>')).toBe(
      'bir\niki\nüç',
    )
  })

  // Chromium's contenteditable makes Enter a new div; treating that as a
  // paragraph would double every line in the plain-text alternative.
  it('reads divs as lines, not paragraphs', () => {
    expect(htmlToText('<div>bir</div><div>iki</div>')).toBe('bir\niki')
  })

  it('is empty for empty HTML', () => {
    expect(htmlToText('')).toBe('')
  })
})

describe('textToHTML', () => {
  it('escapes what looks like markup', () => {
    expect(textToHTML('a < b')).toBe('<p>a &lt; b</p>')
  })

  it('keeps blank lines as paragraph breaks', () => {
    expect(textToHTML('bir\n\niki')).toBe('<p>bir</p><p>iki</p>')
  })

  it('keeps single newlines as line breaks', () => {
    expect(textToHTML('bir\niki')).toBe('<p>bir<br>iki</p>')
  })

  it('round-trips through htmlToText', () => {
    const text = 'bir\niki\n\nüç'
    expect(htmlToText(textToHTML(text))).toBe(text)
  })
})

describe('links', () => {
  it('adds https to a bare host', () => {
    expect(normalizeLinkHref('example.com')).toBe('https://example.com')
  })

  it('reads a bare mailbox as mailto', () => {
    expect(normalizeLinkHref('a@example.com')).toBe('mailto:a@example.com')
  })

  it('refuses a scheme it cannot send', () => {
    expect(normalizeLinkHref('javascript:alert(1)')).toBeNull()
    expect(normalizeLinkHref('data:text/html,hi')).toBeNull()
  })
})

describe('escapeHTML', () => {
  it('escapes the five characters that change meaning', () => {
    expect(escapeHTML(`<&>"'`)).toBe('&lt;&amp;&gt;&quot;&#39;')
  })
})
