import { useEffect, type RefObject } from 'react'

/**
 * Closes an open menu when the reader clicks elsewhere or presses Escape.
 *
 * Both toolbar menus need this and neither is interesting for having it. The
 * listeners are attached only while the menu is open: a mail client keeps one
 * window alive for days, and a pointerdown handler that runs on every click for
 * a menu nobody opened is the kind of thing that is free once and expensive
 * fifteen components later.
 */
export function useDismissOnOutside(
  open: boolean,
  containerRef: RefObject<HTMLElement | null>,
  close: () => void,
) {
  useEffect(() => {
    if (!open) return

    const onPointerDown = (event: PointerEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) {
        close()
      }
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close()
    }

    window.addEventListener('pointerdown', onPointerDown)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('pointerdown', onPointerDown)
      window.removeEventListener('keydown', onKey)
    }
  }, [open, containerRef, close])
}
