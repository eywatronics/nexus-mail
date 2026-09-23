import { useEffect } from 'react'
import {
  applyRead,
  applyStar,
  performRedo,
  performUndo,
  requestDelete,
  selectedIds,
  selectedMessage,
} from './actions'
import { useMailStore } from '../store/useMailStore'

/**
 * True when a keystroke belongs to whatever the user is typing into.
 *
 * Single-key shortcuts and text entry share one keyboard. Without this check,
 * typing the letter j into the search box also jumps the message list, which
 * is the classic way single-key shortcuts get shipped broken.
 */
export function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false

  const tag = target.tagName
  return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable
}

/**
 * Moving through mail without the mouse.
 *
 * j/k are the bindings every mail reader has used since mutt, and the arrow
 * keys are what someone who has never seen those tries first. Both are here
 * because supporting only one of them means half the users conclude the app
 * has no keyboard support at all.
 *
 * A modifier means the keystroke belongs to the window or the platform:
 * Ctrl+ArrowDown is not a request to read the next message.
 *
 * The handler reads the store through getState rather than through the hook's
 * own subscription. Closing over state would re-register the listener on every
 * keystroke that changes the selection, and — worse — a listener installed
 * before the state it reads has settled would act on the stale value.
 */
export function useMessageShortcuts() {
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      // Handled above the guard that drops modified keystrokes, like the redo
      // above it. Uppercase Z is matched as well as lowercase because the
      // event reports the shifted character — but only once the redo has had
      // its look, since Ctrl+Shift+Z is the other direction rather than an
      // undo with the shift key held by accident.
      //
      // Not skipped while typing. An undo aimed at a message and an undo aimed
      // at a search box are the same reflex, and the search box has nothing to
      // undo that losing would matter.
      // Ctrl+Shift+Z and Ctrl+Y both mean redo. Two bindings because the
      // platforms disagree and people carry the habit of whichever they
      // learned first; supporting one of them means half the users conclude
      // there is no redo. Checked before undo, or the shifted Z would be read
      // as an undo with the shift key held by accident.
      if (
        ((event.ctrlKey || event.metaKey) && event.shiftKey && (event.key === 'z' || event.key === 'Z')) ||
        ((event.ctrlKey || event.metaKey) && (event.key === 'y' || event.key === 'Y'))
      ) {
        event.preventDefault()
        void performRedo()
        return
      }

      if ((event.ctrlKey || event.metaKey) && (event.key === 'z' || event.key === 'Z')) {
        event.preventDefault()
        void performUndo()
        return
      }

      // Ctrl+F is the second shortcut that wants a modifier. It opens the
      // reading pane's find bar, which is not the browser's find: the pane is
      // a sandboxed frame the browser's own find cannot see into, so leaving
      // this to the default would give the reader a find that never matched
      // anything in the message they were looking at.
      //
      // Only with a message open. Otherwise it would put a search box on
      // screen with nothing behind it to search.
      if ((event.ctrlKey || event.metaKey) && (event.key === 'f' || event.key === 'F')) {
        if (useMailStore.getState().selectedMessageId === null) return
        event.preventDefault()
        useMailStore.getState().openFind()
        return
      }

      if (event.metaKey || event.ctrlKey || event.altKey) return

      const store = useMailStore.getState()

      // Escape closes the find bar from anywhere, including the list, for the
      // same reason the search box below does: somebody who tabbed away from
      // the box should not have to tab back to shut it. It goes first because
      // the find bar is the nearer of the two — it is over the message the
      // reader is looking at.
      if (event.key === 'Escape' && store.findOpen) {
        event.preventDefault()
        store.closeFind()
        return
      }

      // Escape leaves the search from anywhere, including the list, so a
      // person who tabbed out of the box is not stuck with results.
      if (event.key === 'Escape' && store.searching) {
        event.preventDefault()
        store.clearSearch()
        return
      }

      if (isTypingTarget(event.target)) return

      switch (event.key) {
        case 'j':
        case 'ArrowDown':
          event.preventDefault()
          store.selectRelative(1)
          break
        case 'k':
        case 'ArrowUp':
          event.preventDefault()
          store.selectRelative(-1)
          break

        // The action keys toggle rather than set, so the same key both does
        // and undoes the thing — which is what a reader pressing it twice
        // expects, and what makes it safe to press without looking.
        case 'r': {
          const current = selectedMessage()
          if (!current) break
          event.preventDefault()
          void applyRead(selectedIds(), !current.isRead)
          break
        }
        case 's': {
          const current = selectedMessage()
          if (!current) break
          event.preventDefault()
          void applyStar(selectedIds(), !current.isStarred)
          break
        }

        // In the trash this asks first; everywhere else it moves the message
        // there without a question, because the message is still findable.
        case 'Delete':
        case '#': {
          const ids = selectedIds()
          if (ids.length === 0) break
          event.preventDefault()
          requestDelete(ids)
          break
        }
      }
    }

    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}
