import { CaretDown, CaretRight, MagnifyingGlass, Paperclip, Tray } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useEffect, useMemo, useRef, useState } from 'react'
import { buildRows } from '../lib/threads'
import { useMailStore } from '../store/useMailStore'
import { ICON, ROW_DIVIDER, SELECTED, TEXT, UNSELECTED_BAR } from '../lib/ui'

const ROW_HEIGHT = 74

interface MessageListProps {
  onLoadMore: () => void
}

/**
 * Dates shorten as they age: a message from today needs a time, one from this
 * year needs a day, an older one needs a year. Showing the full date on every
 * row would waste the width the subject needs.
 */
function formatDate(unixSeconds: number): string {
  const date = new Date(unixSeconds * 1000)
  const now = new Date()

  if (date.toDateString() === now.toDateString()) {
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  }
  if (date.getFullYear() === now.getFullYear()) {
    return date.toLocaleDateString(undefined, { day: '2-digit', month: 'short' })
  }
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'short' })
}

/** Shared shell so every empty state sits in the same place on screen. */
function Placeholder({ icon: PlaceholderIcon = Tray, children }: {
  icon?: Icon
  children: React.ReactNode
}) {
  return (
    <div className={`flex h-full flex-col items-center justify-center gap-3 p-8 text-center`}>
      <PlaceholderIcon size={28} weight="light" className={TEXT.muted} />
      <p className={`max-w-[28ch] text-sm ${TEXT.secondary}`}>{children}</p>
    </div>
  )
}

export function MessageList({ onLoadMore }: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null)

  // Search results and a folder's messages are two sources for one column.
  // Which one is showing is decided in the store so this component and the
  // keyboard navigation cannot disagree about what "the next message" means.
  const messages = useMailStore((s) => s.visibleMessages())
  const searching = useMailStore((s) => s.searching)
  const searchPending = useMailStore((s) => s.searchPending)
  const folders = useMailStore((s) => s.folders)

  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const selectedMessageId = useMailStore((s) => s.selectedMessageId)
  const setSelectedMessage = useMailStore((s) => s.setSelectedMessage)
  const hasMore = useMailStore((s) => s.hasMore)
  const loading = useMailStore((s) => s.loadingMessages)

  const threaded = useMailStore((s) => s.threaded)
  const [openThreads, setOpenThreads] = useState<ReadonlySet<string>>(() => new Set())

  // A conversation is open if the reader opened it, or if the selection is
  // inside it. The second half is what keeps the keyboard sane: j and k walk
  // every message, including ones inside a collapsed conversation, and a
  // selection the reader cannot see would be a selection they cannot act on.
  const expanded = useMemo(() => {
    const open = new Set(openThreads)
    const selected = messages.find((m) => m.id === selectedMessageId)
    if (selected) open.add(selected.threadId)
    return open
  }, [openThreads, messages, selectedMessageId])

  const rows = useMemo(
    () => buildRows(messages, threaded && !searching, expanded),
    [messages, threaded, searching, expanded],
  )

  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    // A small overscan keeps scrolling smooth without inflating the DOM, which
    // is the whole point of virtualising in the first place.
    overscan: 8,
  })

  const items = virtualizer.getVirtualItems()
  const lastRenderedIndex = items.length > 0 ? items[items.length - 1].index : -1

  useEffect(() => {
    // Results are a single capped page, so there is nothing further to fetch.
    if (searching || !hasMore || loading || messages.length === 0) return
    if (lastRenderedIndex >= rows.length - 1) {
      onLoadMore()
    }
  }, [searching, hasMore, loading, lastRenderedIndex, rows.length, messages.length, onLoadMore])

  // Keyboard selection has to bring its row with it, or holding j walks the
  // selection off the bottom of the window and the reader loses their place.
  // 'auto' scrolls only when the row is out of view, which leaves a row picked
  // with the mouse exactly where it was clicked.
  //
  // The virtualiser is reached through a ref rather than listed as a
  // dependency: its identity changes on every render, so as a dependency this
  // effect re-runs on every render — including the ones the virtualiser itself
  // triggers while scrolling — and re-scrolls to a row that is already in
  // view. Tracking the last selection scrolled to reduces that to one call per
  // selection change, which is the number of times it means anything.
  const virtualizerRef = useRef(virtualizer)
  virtualizerRef.current = virtualizer
  const lastScrolledTo = useRef<number | null>(null)

  useEffect(() => {
    if (selectedMessageId === null) {
      lastScrolledTo.current = null
      return
    }
    if (lastScrolledTo.current === selectedMessageId) return

    const index = rows.findIndex(
      (row) => row.kind === 'message' && row.message.id === selectedMessageId,
    )
    if (index < 0) return

    lastScrolledTo.current = selectedMessageId
    virtualizerRef.current.scrollToIndex(index, { align: 'auto' })
  }, [selectedMessageId, rows])

  if (searching) {
    if (searchPending && messages.length === 0) {
      return <Placeholder icon={MagnifyingGlass}>Searching…</Placeholder>
    }
    if (messages.length === 0) {
      return (
        <Placeholder icon={MagnifyingGlass}>
          Nothing matches. Only folders that have been opened are searchable.
        </Placeholder>
      )
    }
  } else {
    if (selectedFolderId === null) {
      return <Placeholder>Select a folder to see its messages.</Placeholder>
    }
    if (messages.length === 0) {
      return (
        <Placeholder>
          {loading ? 'Loading messages…' : 'No messages in this folder.'}
        </Placeholder>
      )
    }
  }

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {items.map((item) => {
          const row = rows[item.index]

          if (row.kind === 'thread') {
            return (
              <button
                key={`thread:${row.threadId}`}
                type="button"
                data-testid="thread-row"
                data-thread-count={row.count}
                aria-expanded={row.expanded}
                onClick={() =>
                  setOpenThreads((was) => {
                    const next = new Set(was)
                    if (row.expanded) {
                      next.delete(row.threadId)
                      // The selection lives inside the conversation being
                      // closed, and leaving it there would reopen it on the
                      // next render. Moving it to the header's message is what
                      // makes the close stick.
                      setSelectedMessage(row.newest.id)
                    } else {
                      next.add(row.threadId)
                    }
                    return next
                  })
                }
                className={[
                  'absolute left-0 flex w-full flex-col gap-0.5 px-3 py-2 text-left transition-colors',
                  ROW_DIVIDER,
                  UNSELECTED_BAR,
                  'hover:bg-neutral-100 dark:hover:bg-neutral-900',
                ].join(' ')}
                style={{ top: item.start, height: item.size }}
              >
                <div className="flex items-baseline justify-between gap-2">
                  <span
                    className={`truncate text-sm ${
                      row.unread ? `font-semibold ${TEXT.primary}` : TEXT.secondary
                    }`}
                  >
                    {row.senders.join(', ')}
                  </span>
                  <span className={`tabular shrink-0 font-mono text-xs ${TEXT.muted}`}>
                    {formatDate(row.newest.internalDateUnix)}
                  </span>
                </div>

                <div className="flex items-center gap-1.5">
                  {row.expanded ? (
                    <CaretDown
                      size={ICON.size}
                      weight={ICON.weight}
                      aria-hidden
                      className={`shrink-0 ${TEXT.muted}`}
                    />
                  ) : (
                    <CaretRight
                      size={ICON.size}
                      weight={ICON.weight}
                      aria-hidden
                      className={`shrink-0 ${TEXT.muted}`}
                    />
                  )}
                  <span
                    className={`min-w-0 flex-1 truncate text-sm ${
                      row.unread ? `font-medium ${TEXT.primary}` : TEXT.secondary
                    }`}
                  >
                    {row.newest.subject || '(no subject)'}
                  </span>
                  {/* The count is the one thing a collapsed row says that an
                      ordinary row does not, so it is the one thing drawn
                      differently: monospace, like every other number here. */}
                  <span
                    data-testid="thread-count"
                    className={`tabular shrink-0 font-mono text-xs ${TEXT.muted}`}
                  >
                    {row.count}
                  </span>
                </div>

                <div className={`truncate text-xs ${TEXT.muted}`}>{row.newest.snippet}</div>
              </button>
            )
          }

          const message = row.message
          const selected = message.id === selectedMessageId

          return (
            <button
              key={message.id}
              type="button"
              data-testid="message-row"
              data-unread={!message.isRead}
              data-in-thread={row.inThread || undefined}
              onClick={() => setSelectedMessage(message.id)}
              aria-current={selected}
              className={[
                // Rows are separated by a hairline rather than boxed into
                // cards: at this density, card chrome costs more space than
                // the grouping is worth.
                'absolute left-0 flex w-full flex-col gap-0.5 py-2 pr-3 text-left transition-colors',
                // Messages inside an open conversation are indented instead of
                // boxed, for the same reason: the indent says "part of the
                // thing above" without spending a border on it.
                row.inThread ? 'pl-8' : 'pl-3',
                ROW_DIVIDER,
                selected
                  ? SELECTED
                  : `${UNSELECTED_BAR} hover:bg-neutral-100 dark:hover:bg-neutral-900`,
              ].join(' ')}
              style={{ top: item.start, height: item.size }}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span
                  className={`truncate text-sm ${
                    message.isRead ? TEXT.secondary : `font-semibold ${TEXT.primary}`
                  }`}
                >
                  {message.fromName || message.fromAddr}
                </span>
                <span className={`tabular shrink-0 font-mono text-xs ${TEXT.muted}`}>
                  {formatDate(message.internalDateUnix)}
                </span>
              </div>

              <div className="flex items-center gap-1.5">
                <span
                  className={`min-w-0 flex-1 truncate text-sm ${
                    message.isRead ? TEXT.secondary : `font-medium ${TEXT.primary}`
                  }`}
                >
                  {message.subject || '(no subject)'}
                </span>
                {message.hasAttachments && (
                  <Paperclip
                    size={ICON.size}
                    weight={ICON.weight}
                    aria-label="Has attachments"
                    className={`shrink-0 ${TEXT.muted}`}
                  />
                )}
              </div>

              <div className="flex items-baseline gap-1.5">
                <span className={`min-w-0 flex-1 truncate text-xs ${TEXT.muted}`}>
                  {message.snippet}
                </span>

                {/* Results cross folders, so without this a row cannot say
                    where it came from. Muted rather than a coloured badge: a
                    status colour used for emphasis is a second accent. */}
                {searching && (
                  <span
                    data-testid="result-folder"
                    className={`shrink-0 text-xs ${TEXT.muted}`}
                  >
                    {folders.find((f) => f.id === message.folderId)?.name ?? ''}
                  </span>
                )}
              </div>
            </button>
          )
        })}
      </div>
    </div>
  )
}
