import { describe, expect, it } from 'vitest'
import {
  addressIn,
  formatRecipient,
  formatRecipientLine,
  pillLabel,
  splitRecipientLine,
} from './recipients'

describe('splitRecipientLine', () => {
  it('splits on commas and semicolons', () => {
    expect(splitRecipientLine('a@example.com, b@example.com; c@example.com')).toEqual([
      'a@example.com',
      'b@example.com',
      'c@example.com',
    ])
  })

  // A corporate directory writes "Kabatepe, İsmet"; the comma belongs to the
  // name, not to the address list.
  it('leaves a comma inside a quoted name alone', () => {
    expect(splitRecipientLine('"Kabatepe, İsmet" <a@example.com>, b@example.com')).toEqual([
      '"Kabatepe, İsmet" <a@example.com>',
      'b@example.com',
    ])
  })

  it('leaves a comma inside angle brackets alone', () => {
    expect(splitRecipientLine('Ali <a@example.com>, b@example.com')).toEqual([
      'Ali <a@example.com>',
      'b@example.com',
    ])
  })

  // The empty tail is what says "the thing before the separator is finished".
  it('keeps a trailing empty entry', () => {
    expect(splitRecipientLine('a@example.com, ')).toEqual(['a@example.com', ''])
    expect(splitRecipientLine('a@example.com;')).toEqual(['a@example.com', ''])
  })

  it('returns one empty entry for empty input', () => {
    expect(splitRecipientLine('')).toEqual([''])
  })
})

describe('addressIn', () => {
  it('pulls the address out of a display form', () => {
    expect(addressIn('"Kabatepe, İsmet" <a@example.com>')).toBe('a@example.com')
    expect(addressIn('a@example.com')).toBe('a@example.com')
  })
})

describe('pillLabel', () => {
  it('shows the display name when there is one', () => {
    expect(pillLabel('Alice <alice@example.com>')).toBe('Alice')
  })

  it('unquotes a name that needed quoting', () => {
    expect(pillLabel('"Kabatepe, İsmet" <a@example.com>')).toBe('Kabatepe, İsmet')
  })

  it('falls back to the address', () => {
    expect(pillLabel('a@example.com')).toBe('a@example.com')
    expect(pillLabel('<a@example.com>')).toBe('a@example.com')
  })
})

describe('formatRecipient', () => {
  it('uses the plain display name when it needs no quoting', () => {
    expect(formatRecipient('Alice', 'alice@example.com')).toBe('Alice <alice@example.com>')
  })

  it('quotes a name containing a comma', () => {
    expect(formatRecipient('Kabatepe, İsmet', 'a@example.com')).toBe(
      '"Kabatepe, İsmet" <a@example.com>',
    )
  })

  it('escapes a quote in the display name', () => {
    expect(formatRecipient('Ali "the boss"', 'a@example.com')).toBe(
      '"Ali \\"the boss\\"" <a@example.com>',
    )
  })

  it('falls back to the address when there is no name', () => {
    expect(formatRecipient('', 'a@example.com')).toBe('a@example.com')
  })
})

describe('formatRecipientLine', () => {
  it('joins committed addresses and the unfinished one', () => {
    expect(formatRecipientLine(['Alice <a@example.com>'], 'b@exa')).toBe(
      'Alice <a@example.com>, b@exa',
    )
  })

  it('leaves no trailing separator behind', () => {
    expect(formatRecipientLine(['a@example.com'], '   ')).toBe('a@example.com')
  })
})
