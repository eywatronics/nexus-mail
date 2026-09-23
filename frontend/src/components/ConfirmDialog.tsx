import { Warning } from '@phosphor-icons/react'
import { useEffect, useRef } from 'react'
import { BUTTON_DANGER, BUTTON_SECONDARY, RADIUS, SURFACE, TEXT } from '../lib/ui'

interface ConfirmDialogProps {
  title: string
  body: string
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
}

/**
 * A stop before something that cannot be undone.
 *
 * Used sparingly and on purpose. A client that asks "are you sure" about
 * ordinary actions teaches people to dismiss the question without reading it,
 * which is exactly the habit that makes the one question that mattered useless.
 *
 * Cancel takes the focus, not confirm. Somebody who arrived here by pressing a
 * key twice, or by reflex, gets the harmless outcome from the next reflex —
 * Enter and Escape both back out.
 */
export function ConfirmDialog({
  title,
  body,
  confirmLabel,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    cancelRef.current?.focus()

    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        onCancel()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onCancel])

  return (
    <div
      // The backdrop is a click target as well as a scrim: clicking outside a
      // dialog to dismiss it is what everyone tries first.
      data-testid="confirm-backdrop"
      onClick={onCancel}
      className="fixed inset-0 z-50 flex items-center justify-center bg-neutral-950/40 p-6"
    >
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        aria-describedby="confirm-body"
        data-testid="confirm-dialog"
        onClick={(event) => event.stopPropagation()}
        className={`w-full max-w-sm border p-5 shadow-xl ${RADIUS} ${SURFACE.divider} ${SURFACE.page}`}
      >
        <div className="flex gap-3">
          <Warning
            size={20}
            weight="fill"
            aria-hidden
            className="mt-0.5 shrink-0 text-[var(--color-danger)] dark:text-[var(--color-danger-dark)]"
          />
          <div className="min-w-0">
            <h2 id="confirm-title" className={`text-sm font-semibold ${TEXT.primary}`}>
              {title}
            </h2>
            <p id="confirm-body" className={`mt-1.5 text-sm leading-relaxed ${TEXT.secondary}`}>
              {body}
            </p>
          </div>
        </div>

        {/* Cancel first in the DOM so tabbing reaches the harmless one first,
            and last visually where the confirming button is expected. */}
        <div className="mt-5 flex justify-end gap-2">
          <button
            ref={cancelRef}
            type="button"
            data-testid="confirm-cancel"
            onClick={onCancel}
            className={`${BUTTON_SECONDARY} py-1.5`}
          >
            Cancel
          </button>
          <button
            type="button"
            data-testid="confirm-accept"
            onClick={onConfirm}
            className={`${BUTTON_DANGER} py-1.5`}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
