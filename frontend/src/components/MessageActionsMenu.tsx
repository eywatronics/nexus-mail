import { Code, FileArrowDown } from '@phosphor-icons/react'
import { DotsThreeVertical } from '@phosphor-icons/react'
import { useCallback, useRef, useState } from 'react'
import { saveMessageAsEML } from '../lib/api'
import { useDismissOnOutside } from '../lib/menu'
import { BUTTON_GHOST, ICON, ICON_ONLY, MENU_ITEM, MENU_PANEL } from '../lib/ui'

interface MessageActionsMenuProps {
  messageId: number
  showingSource: boolean
  onToggleSource: () => void
}

/**
 * The things a reader does to a message occasionally.
 *
 * Read, star, move and delete earn their own buttons because they are done
 * constantly. These are not: viewing the source or saving an .eml happens when
 * something has gone wrong, or when a message has to leave the app. Giving
 * each of them a button would push the toolbar past the subject line it sits
 * next to, for controls almost nobody presses on an ordinary day.
 */
export function MessageActionsMenu({
  messageId,
  showingSource,
  onToggleSource,
}: MessageActionsMenuProps) {
  const [open, setOpen] = useState(false)
  const [saved, setSaved] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useDismissOnOutside(open, containerRef, useCallback(() => setOpen(false), []))

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        data-testid="open-message-actions"
        aria-label="More actions"
        aria-expanded={open}
        title="More actions"
        onClick={() => setOpen((was) => !was)}
        className={`${BUTTON_GHOST} ${ICON_ONLY}`}
      >
        <DotsThreeVertical size={ICON.size} weight={ICON.weight} aria-hidden />
      </button>

      {open && (
        <div role="menu" data-testid="message-actions-menu" className={`${MENU_PANEL} w-56`}>
          <button
            type="button"
            role="menuitem"
            data-testid="toggle-source"
            onClick={() => {
              setOpen(false)
              onToggleSource()
            }}
            className={MENU_ITEM}
          >
            <Code size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
            <span>{showingSource ? 'Show message' : 'View source'}</span>
          </button>

          <button
            type="button"
            role="menuitem"
            data-testid="save-eml"
            onClick={() => {
              // The menu stays open so the reader sees the confirmation. A
              // save that closes the menu and opens a file manager behind the
              // window looks like nothing happened at all.
              setSaved(false)
              saveMessageAsEML(messageId)
                .then(() => setSaved(true))
                .catch(() => {})
            }}
            className={MENU_ITEM}
          >
            <FileArrowDown size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
            <span>{saved ? 'Saved' : 'Save as .eml'}</span>
          </button>
        </div>
      )}
    </div>
  )
}
