import { useEffect } from 'react'
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
      if (event.metaKey || event.ctrlKey || event.altKey) return

      const store = useMailStore.getState()

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
      }
    }

    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
}
