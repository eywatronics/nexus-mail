import type { Message } from './api'

/** One line in the message list: a message, or a conversation's header. */
export type ListRow =
  | { kind: 'message'; message: Message; inThread: boolean }
  | {
      kind: 'thread'
      threadId: string
      /** The newest message: its date is what the conversation sorts by, and
       *  its subject is the current one. */
      newest: Message
      count: number
      senders: string[]
      unread: boolean
      expanded: boolean
    }

/**
 * Turns a page of messages into the rows the list draws.
 *
 * The backend returns a conversation's messages adjacent to each other, so
 * grouping is a walk over runs rather than a sort — which matters because the
 * page is only part of the folder, and anything that had to see the whole
 * folder to group correctly would group the first page wrongly.
 *
 * An expanded conversation keeps its header row rather than dissolving into
 * its messages. Without it there is nothing left to click to close the thing
 * again, and the reader has no way back to the list they had.
 *
 * A conversation of one message is never a group: a disclosure control next to
 * a single message is chrome with nothing behind it.
 */
export function buildRows(
  messages: Message[],
  threaded: boolean,
  expanded: ReadonlySet<string>,
): ListRow[] {
  if (!threaded) {
    return messages.map((message) => ({ kind: 'message', message, inThread: false }))
  }

  const rows: ListRow[] = []
  for (let i = 0; i < messages.length; ) {
    const threadId = messages[i].threadId
    let end = i
    while (end < messages.length && messages[end].threadId === threadId) end++

    const run = messages.slice(i, end)
    // The conversation's size in the folder, which can exceed what this page
    // holds: a thread split across a page boundary still knows how big it is.
    const count = Math.max(run[0].threadCount, run.length)

    if (count < 2) {
      rows.push({ kind: 'message', message: run[0], inThread: false })
      i = end
      continue
    }

    const isOpen = expanded.has(threadId)
    rows.push({
      kind: 'thread',
      threadId,
      newest: run[run.length - 1],
      count,
      senders: distinctSenders(run),
      unread: run.some((m) => !m.isRead),
      expanded: isOpen,
    })
    if (isOpen) {
      rows.push(...run.map((message) => ({ kind: 'message' as const, message, inThread: true })))
    }
    i = end
  }
  return rows
}

/**
 * Who is in the conversation, in the order they first appear, without
 * repeating anyone.
 *
 * A collapsed row has one line for the sender, and a five-message exchange
 * between two people would otherwise read "Zeynep, Ahmet, Zeynep, Ahmet,
 * Zeynep" — longer than the two names it is trying to convey.
 */
function distinctSenders(run: Message[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const m of run) {
    const name = m.fromName || m.fromAddr
    if (name && !seen.has(name)) {
      seen.add(name)
      out.push(name)
    }
  }
  return out
}
