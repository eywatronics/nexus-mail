import { ArrowUUpLeft } from '@phosphor-icons/react'
import { performUndo } from '../lib/actions'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST, ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'

const WORDING: Record<string, (count: number) => string> = {
  trash: (n) => (n === 1 ? 'Moved to trash' : `${n} messages moved to trash`),
  move: (n) => (n === 1 ? 'Message moved' : `${n} messages moved`),
  delete: (n) => (n === 1 ? 'Message deleted' : `${n} messages deleted`),
}

/**
 * The few seconds in which a delete can be taken back.
 *
 * It says what happened as well as offering the way out. "Undo" on its own
 * asks the reader to remember what they just did, and the case this exists for
 * is precisely the one where they did something they did not mean to.
 *
 * Anchored to the window rather than placed in a column, because the action it
 * refers to can come from any of them — and because a strip that appeared in
 * the list would push the list down at the exact moment the reader is looking
 * at where the message used to be.
 */
export function UndoNotice() {
  const offer = useMailStore((s) => s.undoOffer)

  if (offer === null || offer.kind === '') return null

  const describe = WORDING[offer.kind]
  if (!describe) return null

  return (
    <div
      data-testid="undo-notice"
      role="status"
      className={[
        RADIUS,
        'fixed bottom-4 left-1/2 z-40 flex -translate-x-1/2 items-center gap-3',
        'border px-3 py-2 shadow-lg',
        SURFACE.divider,
        SURFACE.panel,
      ].join(' ')}
    >
      <span className={`text-sm ${TEXT.secondary}`}>{describe(offer.count)}</span>
      <button
        type="button"
        data-testid="undo-action"
        onClick={() => void performUndo()}
        className={`${BUTTON_GHOST} inline-flex items-center gap-1.5 px-2 py-1 text-sm font-medium`}
      >
        <ArrowUUpLeft size={ICON.size} weight={ICON.weight} aria-hidden />
        Undo
      </button>
    </div>
  )
}
