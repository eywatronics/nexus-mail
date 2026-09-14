import { CaretLeft, Code, DotsThreeVertical, FileArrowDown, TextAa } from '@phosphor-icons/react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { repairCharsets, repairEncoding, type Charset } from '../lib/api'
import { saveMessageAsEML } from '../lib/api'
import { useDismissOnOutside } from '../lib/menu'
import { BUTTON_GHOST, ICON, ICON_ONLY, MENU_ITEM, MENU_PANEL, TEXT } from '../lib/ui'

interface MessageActionsMenuProps {
  messageId: number
  showingSource: boolean
  onToggleSource: () => void
  /** Called after a repair, so the pane reloads the body that just changed. */
  onRepaired: () => void
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
  onRepaired,
}: MessageActionsMenuProps) {
  const [open, setOpen] = useState(false)
  const [saved, setSaved] = useState(false)
  const [pickingCharset, setPickingCharset] = useState(false)
  const [charsets, setCharsets] = useState<Charset[]>([])
  const containerRef = useRef<HTMLDivElement>(null)

  const close = useCallback(() => {
    setOpen(false)
    setPickingCharset(false)
  }, [])
  useDismissOnOutside(open, containerRef, close)

  // Fetched once the menu is first opened rather than on mount. Every message
  // in the list would otherwise ask the backend for a list that never changes.
  useEffect(() => {
    if (!open || charsets.length > 0) return

    let current = true
    repairCharsets()
      .then((list) => {
        if (current) setCharsets(list)
      })
      .catch(() => {})
    return () => {
      current = false
    }
  }, [open, charsets.length])

  // A different message means a different body, so the confirmation on the
  // save item has nothing to confirm any more.
  useEffect(() => {
    setSaved(false)
    setPickingCharset(false)
  }, [messageId])

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
        <div role="menu" data-testid="message-actions-menu" className={`${MENU_PANEL} w-64`}>
          {pickingCharset ? (
            <>
              <button
                type="button"
                role="menuitem"
                data-testid="charset-back"
                onClick={() => setPickingCharset(false)}
                className={MENU_ITEM}
              >
                <CaretLeft size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
                <span>Back</span>
              </button>

              <p className={`px-2 pb-1 pt-2 text-xs ${TEXT.muted}`}>
                Read this message as
              </p>

              {charsets.map((charset) => (
                <button
                  key={charset.name}
                  type="button"
                  role="menuitem"
                  data-testid="charset-option"
                  onClick={() => {
                    close()
                    repairEncoding(messageId, charset.name)
                      .then(onRepaired)
                      .catch(() => {})
                  }}
                  className={MENU_ITEM}
                >
                  <span className="truncate">{charset.label}</span>
                </button>
              ))}
            </>
          ) : (
            <>
              <button
                type="button"
                role="menuitem"
                data-testid="toggle-source"
                onClick={() => {
                  close()
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
                data-testid="open-charset-picker"
                aria-haspopup="menu"
                onClick={() => setPickingCharset(true)}
                className={MENU_ITEM}
              >
                <TextAa size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
                <span>Repair text encoding</span>
              </button>

              <button
                type="button"
                role="menuitem"
                data-testid="save-eml"
                onClick={() => {
                  // The menu stays open so the reader sees the confirmation. A
                  // save that closes the menu and opens a file manager behind
                  // the window looks like nothing happened at all.
                  setSaved(false)
                  saveMessageAsEML(messageId)
                    .then(() => setSaved(true))
                    .catch(() => {})
                }}
                className={MENU_ITEM}
              >
                <FileArrowDown
                  size={ICON.size}
                  weight={ICON.weight}
                  aria-hidden
                  className="shrink-0"
                />
                <span>{saved ? 'Saved' : 'Save as .eml'}</span>
              </button>
            </>
          )}
        </div>
      )}

    </div>
  )
}
