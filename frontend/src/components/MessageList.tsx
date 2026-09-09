import { useVirtualizer } from '@tanstack/react-virtual'
import { useEffect, useRef } from 'react'
import { useMailStore } from '../store/useMailStore'

const ROW_HEIGHT = 74

interface MessageListProps {
  onLoadMore: () => void
}

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
    return (
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        Select a folder to see its messages.
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        {loading ? 'Loading…' : 'No messages in this folder.'}
      </div>
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
              className={`absolute left-0 flex w-full flex-col gap-0.5 border-b border-neutral-100 px-3 py-2 text-left dark:border-neutral-800 ${
                selected
                  ? 'bg-blue-50 dark:bg-blue-950'
                  : 'hover:bg-neutral-50 dark:hover:bg-neutral-900'
              }`}
              style={{ top: item.start, height: item.size }}
            >
              <div className="flex items-baseline justify-between gap-2">
                <span
                  className={`truncate text-sm ${
                    message.isRead
                      ? 'text-neutral-600 dark:text-neutral-400'
                      : 'font-semibold text-neutral-900 dark:text-neutral-100'
                  }`}
                >
                  {message.fromName || message.fromAddr}
                </span>
                <span className="shrink-0 text-xs text-neutral-400">
                  {formatDate(message.internalDateUnix)}
                </span>
              </div>

              <span
                className={`truncate text-sm ${
                  message.isRead ? 'text-neutral-600 dark:text-neutral-400' : 'font-medium'
                }`}
              >
                {message.subject || '(no subject)'}
                {message.hasAttachments && (
                  <span aria-label="Has attachments" className="ml-1 text-neutral-400">
                    📎
                  </span>
                )}
              </span>

              <span className="truncate text-xs text-neutral-400">{message.snippet}</span>
            </button>
          )
        })}
      </div>
    </div>
  )
}
