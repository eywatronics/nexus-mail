import { describe, expect, it } from 'vitest'
import { mentionsAttachment, textBeforeQuote } from './attachmentReminder'

describe('mentionsAttachment', () => {
  it.each([
    'Please find the attached report.',
    'See attachment.',
    'I have attached three files.',
    'The enclosed invoice is ready.',
    'Ekte gönderiyorum.',
    'Ekli dosyayı inceleyin.',
    'İlişikteki belgeyi gönderiyorum.',
    'Bir ek daha var.',
  ])('recognises %s', (text) => {
    expect(mentionsAttachment(text)).toBe(true)
  })

  // "ek" is a word, but it is also the first syllable of "ekmek" and "ekran";
  // a reminder on every message containing either would train the writer to
  // dismiss it without reading, which is the one outcome this must avoid.
  it.each([
    'No file here.',
    'Ekmek tarifi gönderiyorum.',
    'Ekran görüntüsü yok.',
    'The handbook covers this.',
    'reattached',
  ])('does not fire on %s', (text) => {
    expect(mentionsAttachment(text)).toBe(false)
  })

  it('matches without caring about case', () => {
    expect(mentionsAttachment('ATTACHED')).toBe(true)
    expect(mentionsAttachment('EKTE')).toBe(true)
  })
})

describe('textBeforeQuote', () => {
  it('drops the original below the reply', () => {
    const quoted = '> See the attached file.'
    expect(textBeforeQuote(`Thanks.\n\n${quoted}`, quoted)).toBe('Thanks.\n\n')
  })

  // If the writer edited the quote, there is no clean boundary. Checking the
  // whole body is the harmless failure: one extra question.
  it('uses the whole body when the quote is not where it was', () => {
    expect(textBeforeQuote('Thanks.', '> Something else.')).toBe('Thanks.')
  })

  it('uses the whole body when there is no quote', () => {
    expect(textBeforeQuote('Attached below.', undefined)).toBe('Attached below.')
  })
})
