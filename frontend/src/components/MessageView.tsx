import { Envelope, EnvelopeOpen, EyeSlash, Star, Trash } from '@phosphor-icons/react'
import { useEffect, useMemo, useState } from 'react'
import { bodyURL } from '../lib/api'
import { applyDelete, applyRead, applyStar } from '../lib/actions'
import { AttachmentList } from './AttachmentList'
import { MoveMenu } from './MoveMenu'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST, BUTTON_SECONDARY, ICON, SURFACE, TEXT } from '../lib/ui'

/**
 * The sandbox attribute is deliberately minimal.
 *
 * allow-same-origin is absent, and that absence is the single most important
 * line in the frontend: granting it would give mail scripts access to this
 * app's origin, including local storage and the Wails bridge, and the
 * isolation would be decorative. allow-scripts is absent too — mail has no
 * legitimate need to run code.
 */
const SANDBOX = 'allow-popups allow-popups-to-escape-sandbox'

/**
 * Selection changes are debounced. Holding j or the arrow keys moves through
 * the list dozens of times a second, and every intermediate message would
 * otherwise be fetched and sanitised for nobody to read.
 */
const SELECTION_DEBOUNCE_MS = 120

/**
 * Full date for the reading pane. The list abbreviates because it has one
 * column to spare; here there is room, and the reader has usually stopped
 * scanning and started reading.
 */
function formatFullDate(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString(undefined, {
    day: '2-digit',
    month: 'long',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function MessageView() {
  const selectedMessageId = useMailStore((s) => s.selectedMessageId)
  const message = useMailStore((s) =>
    s.messages.find((m) => m.id === s.selectedMessageId),
  )
  const [settledId, setSettledId] = useState<number | null>(null)
  const [allowRemote, setAllowRemote] = useState(false)

  useEffect(() => {
    // Consent is per message and never sticky: carrying it forward would
    // silently load trackers in whatever the user opens next.
    setAllowRemote(false)

    if (selectedMessageId === null) {
      setSettledId(null)
      return
    }

    const timer = setTimeout(() => setSettledId(selectedMessageId), SELECTION_DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [selectedMessageId])

  const src = useMemo(
    () => (settledId === null ? null : bodyURL(settledId, allowRemote)),
    [settledId, allowRemote],
  )

  // Reading a message marks it read. Tied to the settled id rather than the
  // selection, so holding j through a folder does not mark fifty messages read
  // on the way past — only the one the reader actually stopped on.
  useEffect(() => {
    if (settledId === null) return

    const current = useMailStore.getState().visibleMessages().find((m) => m.id === settledId)
    if (!current || current.isRead) return

    void applyRead([settledId], true)
  }, [settledId])

  if (selectedMessageId === null) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
        <Envelope size={28} weight="light" className={TEXT.muted} />
        <p className={`max-w-[28ch] text-sm ${TEXT.secondary}`}>
          Select a message to read it.
        </p>
      </div>
    )
  }

  if (src === null) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <p className={`text-sm ${TEXT.secondary}`}>Loading…</p>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      {/* Once the reader is in the body, the list may be scrolled far away.
          Without a header there is nothing on screen saying who wrote this or
          what it is about. */}
      {message && (
        <header className={`border-b px-6 py-4 ${SURFACE.divider}`}>
          <div className="flex items-start gap-3">
            <h1 className={`min-w-0 flex-1 text-base font-semibold leading-snug ${TEXT.primary}`}>
              {message.subject || '(no subject)'}
            </h1>

            {/* The three things a reader does to a message without opening
                anything else. Ghost buttons rather than a bordered toolbar:
                the message is the content here, and chrome around it competes
                with what the reader came for. */}
            <div className="flex shrink-0 items-center gap-0.5">
              <button
                type="button"
                data-testid="toggle-read"
                aria-label={message.isRead ? 'Mark as unread' : 'Mark as read'}
                title={message.isRead ? 'Mark as unread (r)' : 'Mark as read (r)'}
                onClick={() => void applyRead([message.id], !message.isRead)}
                className={`${BUTTON_GHOST} inline-flex items-center p-1.5`}
              >
                {message.isRead ? (
                  <Envelope size={ICON.size} weight={ICON.weight} aria-hidden />
                ) : (
                  <EnvelopeOpen size={ICON.size} weight={ICON.weight} aria-hidden />
                )}
              </button>

              <button
                type="button"
                data-testid="toggle-star"
                aria-label={message.isStarred ? 'Remove star' : 'Add star'}
                title={message.isStarred ? 'Remove star (s)' : 'Add star (s)'}
                onClick={() => void applyStar([message.id], !message.isStarred)}
                className={`${BUTTON_GHOST} inline-flex items-center p-1.5`}
              >
                <Star
                  size={ICON.size}
                  weight={message.isStarred ? 'fill' : ICON.weight}
                  aria-hidden
                  className={message.isStarred ? 'text-[var(--color-accent)]' : undefined}
                />
              </button>

              <MoveMenu messageId={message.id} folderId={message.folderId} />

              <button
                type="button"
                data-testid="delete-message"
                aria-label="Delete"
                title="Delete (Del)"
                onClick={() => {
                  // Move on first, so the reader is left looking at the next
                  // message rather than an empty pane.
                  useMailStore.getState().selectRelative(1)
                  void applyDelete([message.id])
                }}
                className={`${BUTTON_GHOST} inline-flex items-center p-1.5`}
              >
                <Trash size={ICON.size} weight={ICON.weight} aria-hidden />
              </button>
            </div>
          </div>

          <div className="mt-1.5 flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
            <span className={`text-sm ${TEXT.primary}`}>
              {message.fromName || message.fromAddr}
            </span>
            {message.fromName && (
              <span className={`text-xs ${TEXT.muted}`}>{message.fromAddr}</span>
            )}
            <span className={`tabular ml-auto font-mono text-xs ${TEXT.muted}`}>
              {formatFullDate(message.internalDateUnix)}
            </span>
          </div>
        </header>
      )}

      {message?.hasAttachments && <AttachmentList messageId={message.id} />}

      {!allowRemote && (
        <div
          className={`flex items-center justify-between gap-3 border-b px-4 py-2 ${SURFACE.divider} ${SURFACE.panel}`}
        >
          <div className="flex min-w-0 items-center gap-2">
            <EyeSlash
              size={ICON.size}
              weight={ICON.weight}
              aria-hidden
              className={`shrink-0 ${TEXT.muted}`}
            />
            <p className={`min-w-0 text-xs ${TEXT.secondary}`}>
              Remote content is blocked, so the sender cannot tell you opened this
              message.
            </p>
          </div>

          <button
            type="button"
            data-testid="load-remote"
            onClick={() => setAllowRemote(true)}
            className={`${BUTTON_SECONDARY} shrink-0 whitespace-nowrap px-3 py-1 text-xs`}
          >
            Load images
          </button>
        </div>
      )}

      <iframe
        // Keying on the URL forces a fresh document when consent changes,
        // rather than leaving the previous render in place.
        key={src}
        title="Message body"
        sandbox={SANDBOX}
        src={src}
        className="h-full w-full flex-1 border-0"
      />
    </div>
  )
}
