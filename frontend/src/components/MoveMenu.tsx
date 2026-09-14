import { FolderOpen } from '@phosphor-icons/react'
import { useCallback, useRef, useState } from 'react'
import { moveMessages } from '../lib/api'
import { useDismissOnOutside } from '../lib/menu'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST, ICON, MENU_ITEM, MENU_PANEL, ICON_ONLY } from '../lib/ui'

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

  useDismissOnOutside(open, containerRef, useCallback(() => setOpen(false), []))

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
        className={`${BUTTON_GHOST} ${ICON_ONLY}`}
      >
        <FolderOpen size={ICON.size} weight={ICON.weight} aria-hidden />
      </button>

      {open && (
        <div
          role="menu"
          data-testid="move-menu"
          className={`${MENU_PANEL} max-h-64 w-56 overflow-y-auto`}
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
              className={MENU_ITEM}
            >
              <span className="truncate">{folder.name}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
