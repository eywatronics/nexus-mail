import { FolderOpen } from '@phosphor-icons/react'
import { useEffect, useRef, useState } from 'react'
import { moveMessages } from '../lib/api'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST, ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'

interface MoveMenuProps {
  messageId: number
  folderId: number
}

/**
 * Moves a message to another folder.
 *
 * The list is the folders of the account the message is in. Offering another
 * account's folders would be offering something IMAP cannot do in one
 * operation — a cross-account move is a copy, an upload and a delete, and
 * silently doing three things behind one label is how a client loses mail.
 */
export function MoveMenu({ messageId, folderId }: MoveMenuProps) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  const folders = useMailStore((s) => s.folders)
  const removeLocalMessages = useMailStore((s) => s.removeLocalMessages)
  const selectRelative = useMailStore((s) => s.selectRelative)

  const accountId = folders.find((f) => f.id === folderId)?.accountId
  const destinations = folders.filter((f) => f.accountId === accountId && f.id !== folderId)

  useEffect(() => {
    if (!open) return

    // Clicking anywhere else closes it, which is what every menu does and what
    // a reader will try without thinking about it.
    const onPointerDown = (event: PointerEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) {
        setOpen(false)
      }
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }

    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKey)
    }
  }, [open])

  if (destinations.length === 0) return null

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        data-testid="open-move-menu"
        aria-label="Move to folder"
        aria-expanded={open}
        title="Move to folder"
        onClick={() => setOpen((was) => !was)}
        className={`${BUTTON_GHOST} inline-flex items-center p-1.5`}
      >
        <FolderOpen size={ICON.size} weight={ICON.weight} aria-hidden />
      </button>

      {open && (
        <div
          role="menu"
          data-testid="move-menu"
          className={`absolute right-0 top-full z-10 mt-1 max-h-64 w-56 overflow-y-auto border p-1 shadow-lg ${RADIUS} ${SURFACE.divider} ${SURFACE.panel}`}
        >
          {destinations.map((folder) => (
            <button
              key={folder.id}
              type="button"
              role="menuitem"
              data-testid="move-destination"
              onClick={() => {
                setOpen(false)
                // Move the selection on first: the message is about to leave
                // this folder, and leaving the reader on an empty pane after
                // every move would make filing a mailbox a chore.
                selectRelative(1)
                removeLocalMessages([messageId])
                void moveMessages([messageId], folder.id).catch(() => {})
              }}
              className={[
                RADIUS,
                'flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm transition-colors',
                TEXT.secondary,
                'hover:bg-neutral-200/70 dark:hover:bg-neutral-800',
              ].join(' ')}
            >
              <span className="truncate">{folder.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
