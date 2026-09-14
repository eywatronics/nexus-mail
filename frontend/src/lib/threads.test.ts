import { describe, expect, it } from 'vitest'
import { buildRows } from './threads'
import type { Message } from './api'

const message = (
  id: number,
  threadId: string,
  threadCount: number,
  overrides: Partial<Message> = {},
): Message => ({
  id,
  folderId: 1,
  uid: id,
  threadId,
  threadCount,
  subject: `Konu ${id}`,
  fromName: `Gönderen ${id}`,
  fromAddr: `g${id}@example.com`,
  snippet: 'önizleme',
  internalDateUnix: 1700000000 + id,
  isRead: true,
  isStarred: false,
  hasAttachments: false,
  bodyFetched: false,
  ...overrides,
})

const none = new Set<string>()

describe('grouping messages into conversations', () => {
  it('leaves the list flat when grouping is off', () => {
    const rows = buildRows(
      [message(1, '<a>', 2), message(2, '<a>', 2)],
      false,
      none,
    )

    expect(rows.map((r) => r.kind)).toEqual(['message', 'message'])
  })

  // A disclosure control next to a single message is chrome with nothing
  // behind it.
  it('does not group a conversation of one', () => {
    const rows = buildRows([message(1, '<a>', 1)], true, none)

    expect(rows).toHaveLength(1)
    expect(rows[0].kind).toBe('message')
  })

  it('collapses a conversation into one row', () => {
    const rows = buildRows(
      [message(1, '<a>', 2), message(2, '<a>', 2), message(3, '<b>', 1)],
      true,
      none,
    )

    expect(rows.map((r) => r.kind)).toEqual(['thread', 'message'])
    const thread = rows[0]
    if (thread.kind !== 'thread') throw new Error('expected a thread row')
    expect(thread.count).toBe(2)
    // The newest message is what the row is about: its subject is the current
    // one and its date is what the conversation sorts by.
    expect(thread.newest.id).toBe(2)
  })

  // Without a header there is nothing left to click to close the thing again,
  // and the reader has no way back to the list they had.
  it('keeps the header when a conversation is open', () => {
    const rows = buildRows(
      [message(1, '<a>', 2), message(2, '<a>', 2)],
      true,
      new Set(['<a>']),
    )

    expect(rows.map((r) => r.kind)).toEqual(['thread', 'message', 'message'])
    const header = rows[0]
    if (header.kind !== 'thread') throw new Error('expected a thread row')
    expect(header.expanded).toBe(true)
  })

  it('marks an open conversation’s messages as nested', () => {
    const rows = buildRows(
      [message(1, '<a>', 2), message(2, '<a>', 2)],
      true,
      new Set(['<a>']),
    )

    for (const row of rows.slice(1)) {
      if (row.kind !== 'message') throw new Error('expected message rows')
      expect(row.inThread).toBe(true)
    }
  })

  // A thread split across a page boundary still has to say how big it is, or
  // the count would climb as the reader scrolls.
  it('trusts the backend count over what this page holds', () => {
    const rows = buildRows([message(1, '<a>', 9)], true, none)

    const thread = rows[0]
    if (thread.kind !== 'thread') throw new Error('expected a thread row')
    expect(thread.count).toBe(9)
  })

  // And the other way: if more of the conversation is loaded than the stored
  // count claims, the rows on screen are the better answer.
  it('never reports fewer messages than it is showing', () => {
    const rows = buildRows([message(1, '<a>', 1), message(2, '<a>', 1)], true, none)

    const thread = rows[0]
    if (thread.kind !== 'thread') throw new Error('expected a thread row')
    expect(thread.count).toBe(2)
  })

  // A five-message exchange between two people would otherwise read "Zeynep,
  // Ahmet, Zeynep, Ahmet, Zeynep" — longer than the two names it conveys.
  it('lists each participant once, in the order they appear', () => {
    const rows = buildRows(
      [
        message(1, '<a>', 3, { fromName: 'Zeynep' }),
        message(2, '<a>', 3, { fromName: 'Ahmet' }),
        message(3, '<a>', 3, { fromName: 'Zeynep' }),
      ],
      true,
      none,
    )

    const thread = rows[0]
    if (thread.kind !== 'thread') throw new Error('expected a thread row')
    expect(thread.senders).toEqual(['Zeynep', 'Ahmet'])
  })

  // A collapsed conversation has to look unread when any message in it is,
  // otherwise grouping hides new mail.
  it('reports a conversation unread when any message in it is', () => {
    const rows = buildRows(
      [message(1, '<a>', 2), message(2, '<a>', 2, { isRead: false })],
      true,
      none,
    )

    const thread = rows[0]
    if (thread.kind !== 'thread') throw new Error('expected a thread row')
    expect(thread.unread).toBe(true)
  })

  it('does not merge two conversations that happen to be adjacent', () => {
    const rows = buildRows(
      [message(1, '<a>', 2), message(2, '<a>', 2), message(3, '<b>', 2), message(4, '<b>', 2)],
      true,
      none,
    )

    expect(rows).toHaveLength(2)
    const [first, second] = rows
    if (first.kind !== 'thread' || second.kind !== 'thread') {
      throw new Error('expected two thread rows')
    }
    expect(first.threadId).toBe('<a>')
    expect(second.threadId).toBe('<b>')
  })

  it('handles an empty page', () => {
    expect(buildRows([], true, none)).toEqual([])
  })
})
