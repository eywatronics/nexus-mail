import { Paperclip, Tray } from '@phosphor-icons/react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useEffect, useRef } from 'react'
import { useMailStore } from '../store/useMailStore'
import { ICON, SELECTED, SURFACE, TEXT, UNSELECTED_BAR } from '../lib/ui'

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
function Placeholder({ children }: { children: React.ReactNode }) {
  return (
    <div className={`flex h-full flex-col items-center justify-center gap-3 p-8 text-center`}>
      <Tray size={28} weight="light" className={TEXT.muted} />
      <p className={`max-w-[28ch] text-sm ${TEXT.secondary}`}>{children}</p>
    </div>
  )
}

export function MessageList({ onLoadMore }: MessageListProps) {
  const scrollRef = useRef<HTMLDivElement>(null)

  const messages = useMailStore((s) => s.messages)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const selectedMessageId = useMailStore((s) => s.selectedMessageId)
  const setSelectedMessage = useMailStore((s) => s.setSelectedMessage)
  const hasMore = useMailStore((s) => s.hasMore)
  const loading = useMailStore((s) => s.loadingMessages)

  const virtualizer = useVirtualizer({
    count: messages.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT,
    // A small overscan keeps scrolling smooth without inflating the DOM, which
    // is the whole point of virtualising in the first place.
    overscan: 8,
  })

  const items = virtualizer.getVirtualItems()
  const lastRenderedIndex = items.length > 0 ? items[items.length - 1].index : -1

  useEffect(() => {
    if (!hasMore || loading || messages.length === 0) return
    if (lastRenderedIndex >= messages.length - 1) {
      onLoadMore()
    }
  }, [hasMore, loading, lastRenderedIndex, messages.length, onLoadMore])

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

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {items.map((item) => {
          const message = messages[item.index]
          const selected = message.id === selectedMessageId

          return (
            <button
              key={message.id}
              type="button"
              data-testid="message-row"
              data-unread={!message.isRead}
              onClick={() => setSelectedMessage(message.id)}
              aria-current={selected}
              className={[
                // Rows are separated by a hairline rather than boxed into
                // cards: at this density, card chrome costs more space than
                // the grouping is worth.
                'absolute left-0 flex w-full flex-col gap-0.5 border-b px-3 py-2 text-left transition-colors',
                SURFACE.divider,
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

              <span className={`truncate text-xs ${TEXT.muted}`}>{message.snippet}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
