import { Envelope, EyeSlash } from '@phosphor-icons/react'
import { useEffect, useMemo, useState } from 'react'
import { bodyURL } from '../lib/api'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_SECONDARY, ICON, SURFACE, TEXT } from '../lib/ui'

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
          <h1 className={`text-base font-semibold leading-snug ${TEXT.primary}`}>
            {message.subject || '(no subject)'}
          </h1>

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
