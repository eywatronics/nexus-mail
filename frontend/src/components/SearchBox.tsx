import { ChatsCircle, CircleNotch, MagnifyingGlass, X } from '@phosphor-icons/react'
import { useEffect, useRef } from 'react'
import { useMailStore } from '../store/useMailStore'
import { EmptyTrashButton } from './EmptyTrashButton'
import { isTypingTarget } from '../lib/keyboard'
import { BUTTON_GHOST, ICON, ICON_ONLY, INPUT, SURFACE, TEXT } from '../lib/ui'

/**
 * The header strip of the message column.
 *
 * It sits here rather than in the sidebar because this column is what its
 * controls change. A control in one column that silently rewrites another is
 * how a person ends up not noticing the search is still open.
 *
 * The conversation toggle shares the strip rather than getting a bar of its
 * own: this is a dense interface, and a second row of chrome costs the list a
 * message's worth of height for one button.
 */
export function SearchBox() {
  const inputRef = useRef<HTMLInputElement>(null)

  const searchQuery = useMailStore((s) => s.searchQuery)
  const searchPending = useMailStore((s) => s.searchPending)
  const setSearchQuery = useMailStore((s) => s.setSearchQuery)
  const clearSearch = useMailStore((s) => s.clearSearch)
  const threaded = useMailStore((s) => s.threaded)
  const setThreaded = useMailStore((s) => s.setThreaded)
  const searching = useMailStore((s) => s.searching)

  useEffect(() => {
    // "/" is the shortcut every reader-shaped application uses, and it has to
    // reach the box from anywhere. Ignoring it while a field already has focus
    // keeps it typeable in the box itself.
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      if (isTypingTarget(event.target)) return

      event.preventDefault()
      inputRef.current?.focus()
      inputRef.current?.select()
    }

    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  return (
    <div className={`flex items-center gap-1.5 border-b p-2 ${SURFACE.divider}`}>
      <div className="relative min-w-0 flex-1">
        <MagnifyingGlass
          size={ICON.size}
          weight={ICON.weight}
          aria-hidden
          className={`pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 ${TEXT.muted}`}
        />

        <input
          ref={inputRef}
          type="search"
          role="searchbox"
          aria-label="Search mail"
          placeholder="Search mail"
          value={searchQuery}
          onChange={(event) => setSearchQuery(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Escape') {
              clearSearch()
              inputRef.current?.blur()
            }
          }}
          // The native clear affordance is suppressed: it varies by platform,
          // it is not keyboard reachable in every engine, and there is an
          // explicit button beside it.
          className={`${INPUT} w-full py-1.5 pl-8 pr-8 text-sm [&::-webkit-search-cancel-button]:hidden`}
        />

        {searchPending && (
          <CircleNotch
            size={ICON.size}
            weight={ICON.weight}
            data-testid="search-pending"
            aria-label="Searching"
            className={`absolute right-2.5 top-1/2 -translate-y-1/2 animate-spin ${TEXT.muted}`}
          />
        )}

        {!searchPending && searchQuery !== '' && (
          <button
            type="button"
            data-testid="clear-search"
            aria-label="Clear search"
            onClick={() => {
              clearSearch()
              inputRef.current?.focus()
            }}
            className={`absolute right-1.5 top-1/2 -translate-y-1/2 rounded-[var(--radius-ui)] p-1 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-800 ${TEXT.muted}`}
          >
            <X size={ICON.size} weight={ICON.weight} aria-hidden />
          </button>
        )}
      </div>

      {/* Only drawn inside the trash, which is the only place it means
          anything. */}
      <EmptyTrashButton />

      {/* Disabled while searching rather than hidden: results span folders and
          are ranked by relevance, so there is nothing to group. A control that
          vanished would leave the reader wondering where it went. */}
      <button
        type="button"
        data-testid="toggle-threading"
        aria-pressed={threaded}
        disabled={searching}
        aria-label={threaded ? 'Show messages individually' : 'Group by conversation'}
        title={threaded ? 'Show messages individually' : 'Group by conversation'}
        onClick={() => setThreaded(!threaded)}
        className={[
          BUTTON_GHOST,
          ICON_ONLY,
          'shrink-0 disabled:cursor-not-allowed disabled:opacity-40',
          threaded ? 'text-[var(--color-accent)]' : '',
        ].join(' ')}
      >
        <ChatsCircle
          size={ICON.size}
          weight={threaded ? 'fill' : ICON.weight}
          aria-hidden
        />
      </button>
    </div>
  )
}
