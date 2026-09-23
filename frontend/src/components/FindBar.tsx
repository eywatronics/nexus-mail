import { CaretDown, CaretUp, X } from '@phosphor-icons/react'
import { useEffect, useRef } from 'react'
import { BUTTON_GHOST, ICON, ICON_ONLY, INPUT, SURFACE, TEXT } from '../lib/ui'

interface FindBarProps {
  query: string
  onQueryChange: (next: string) => void
  /** How many matches the served document held, or null before it has answered. */
  count: number | null
  /** Which match is showing, zero-based. */
  index: number
  onStep: (delta: number) => void
  onClose: () => void
}

/**
 * Find within the open message.
 *
 * The searching itself happens in the backend, not here. The reading pane is a
 * sandboxed frame with no scripts and no shared origin, so neither the
 * browser's own find nor any script of ours can reach inside it; the query
 * travels in the body URL and comes back as a document with the matches
 * already wrapped. That is why this bar has no "highlight all" switch — every
 * match is always marked, because marking them is how the search is done.
 *
 * It sits above the message rather than floating over it, which is what a
 * browser does. A floating bar covers the text it is searching, and in a pane
 * this narrow it would cover a useful fraction of it.
 */
export function FindBar({
  query,
  onQueryChange,
  count,
  index,
  onStep,
  onClose,
}: FindBarProps) {
  const input = useRef<HTMLInputElement>(null)

  // Opening the bar puts the cursor in it. A find bar that needs to be clicked
  // before it can be typed into is one the keyboard shortcut only half opened.
  useEffect(() => {
    input.current?.focus()
    input.current?.select()
  }, [])

  const searching = query.trim() !== ''
  const hasMatches = count !== null && count > 0
  const missed = searching && count === 0

  const onKeyDown = (event: React.KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    // Enter for the next match and Shift+Enter for the previous one is what
    // every find bar has done for twenty years, and it is what somebody who
    // never looks at the arrows will try.
    if (event.key === 'Enter') {
      event.preventDefault()
      onStep(event.shiftKey ? -1 : 1)
    }
  }

  return (
    <div
      role="search"
      className={`flex items-center gap-2 border-b px-4 py-2 ${SURFACE.divider} ${SURFACE.panel}`}
    >
      <input
        ref={input}
        type="text"
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        onKeyDown={onKeyDown}
        aria-label="Find in message"
        placeholder="Find in message"
        className={`${INPUT} min-w-0 flex-1 py-1 text-sm`}
      />

      {/* The count is announced rather than only drawn: a reader on a screen
          reader gets the same "nothing found" the sighted one does. */}
      <span
        role="status"
        aria-live="polite"
        data-testid="find-count"
        className={`tabular shrink-0 font-mono text-xs ${missed ? TEXT.secondary : TEXT.muted}`}
      >
        {!searching ? '' : count === null ? '…' : hasMatches ? `${index + 1}/${count}` : 'No matches'}
      </span>

      <div className="flex shrink-0 items-center gap-0.5">
        <button
          type="button"
          aria-label="Previous match"
          title="Previous match (Shift+Enter)"
          disabled={!hasMatches}
          onClick={() => onStep(-1)}
          className={`${BUTTON_GHOST} ${ICON_ONLY} disabled:cursor-not-allowed disabled:opacity-40`}
        >
          <CaretUp size={ICON.size} weight={ICON.weight} aria-hidden />
        </button>
        <button
          type="button"
          aria-label="Next match"
          title="Next match (Enter)"
          disabled={!hasMatches}
          onClick={() => onStep(1)}
          className={`${BUTTON_GHOST} ${ICON_ONLY} disabled:cursor-not-allowed disabled:opacity-40`}
        >
          <CaretDown size={ICON.size} weight={ICON.weight} aria-hidden />
        </button>
        <button
          type="button"
          aria-label="Close find bar"
          title="Close (Esc)"
          onClick={onClose}
          className={`${BUTTON_GHOST} ${ICON_ONLY}`}
        >
          <X size={ICON.size} weight={ICON.weight} aria-hidden />
        </button>
      </div>
    </div>
  )
}
