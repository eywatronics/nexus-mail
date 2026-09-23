import { ArrowUUpLeft, ArrowUUpRight } from '@phosphor-icons/react'
import { performRedo, performUndo } from '../lib/actions'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST, ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'

const UNDO_WORDING: Record<string, (count: number) => string> = {
  trash: (n) => (n === 1 ? 'Moved to trash' : `${n} messages moved to trash`),
  move: (n) => (n === 1 ? 'Message moved' : `${n} messages moved`),
  delete: (n) => (n === 1 ? 'Message deleted' : `${n} messages deleted`),
}

/**
 * What an undo did, so the way back says where it would go.
 *
 * "Undone" on its own is no better than a bare "Undo": it tells the reader
 * something changed without saying what, in the one moment they are least
 * sure.
 */
const REDO_WORDING: Record<string, (count: number) => string> = {
  trash: (n) => (n === 1 ? 'Taken back out of the trash' : `${n} messages taken back`),
  move: (n) => (n === 1 ? 'Move undone' : `${n} moves undone`),
  delete: (n) => (n === 1 ? 'Deletion undone' : `${n} deletions undone`),
}

/**
 * The few seconds in which a delete can be taken back — and then put back.
 *
 * It says what happened as well as offering the way out. "Undo" on its own
 * asks the reader to remember what they just did, and the case this exists for
 * is precisely the one where they did something they did not mean to.
 *
 * Anchored to the window rather than placed in a column, because the action it
 * refers to can come from any of them — and because a strip that appeared in
 * the list would push the list down at the exact moment the reader is looking
 * at where the message used to be.
 *
 * Taking the undo turns the strip into its opposite rather than dismissing it.
 * The reader who undid by reflex and then thought better of it is the same
 * reader this exists for, one step further along, and making them redo the
 * action by hand would be a strange place to stop helping.
 */
export function UndoNotice() {
  const undoOffer = useMailStore((s) => s.undoOffer)
  const redoOffer = useMailStore((s) => s.redoOffer)

  // Undo first. The two are never live together — taking an undo is what
  // creates a redo — but if a race ever made them overlap, the more recent
  // action is the one the reader is thinking about.
  const showing = undoOffer && undoOffer.kind !== '' ? 'undo' : 'redo'
  const offer = showing === 'undo' ? undoOffer : redoOffer

  if (!offer || offer.kind === '') return null

  const describe = (showing === 'undo' ? UNDO_WORDING : REDO_WORDING)[offer.kind]
  if (!describe) return null

  const undoing = showing === 'undo'
  const Icon = undoing ? ArrowUUpLeft : ArrowUUpRight

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
        data-testid={undoing ? 'undo-action' : 'redo-action'}
        onClick={() => void (undoing ? performUndo() : performRedo())}
        className={`${BUTTON_GHOST} inline-flex items-center gap-1.5 px-2 py-1 text-sm font-medium`}
      >
        <Icon size={ICON.size} weight={ICON.weight} aria-hidden />
        {undoing ? 'Undo' : 'Redo'}
      </button>
    </div>
  )
}
