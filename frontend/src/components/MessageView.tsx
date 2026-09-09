import { useEffect, useMemo, useState } from 'react'
import { bodyURL } from '../lib/api'
import { useMailStore } from '../store/useMailStore'

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

export function MessageView() {
  const selectedMessageId = useMailStore((s) => s.selectedMessageId)
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
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        Select a message to read it.
      </div>
    )
  }

  if (src === null) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center text-sm text-neutral-500">
        Loading…
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      {!allowRemote && (
        <div className="flex items-center justify-between gap-3 border-b border-amber-200 bg-amber-50 px-4 py-2 text-xs dark:border-amber-900 dark:bg-amber-950">
          <span className="text-amber-900 dark:text-amber-200">
            Remote content is blocked so the sender cannot learn that you opened this
            message.
          </span>
          <button
            type="button"
            data-testid="load-remote"
            onClick={() => setAllowRemote(true)}
            className="shrink-0 rounded border border-amber-400 px-2 py-1 font-medium text-amber-900 hover:bg-amber-100 dark:text-amber-200 dark:hover:bg-amber-900"
          >
            Load remote content
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
