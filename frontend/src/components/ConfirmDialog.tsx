import { Paperclip, Warning } from '@phosphor-icons/react'
import { useEffect, useRef } from 'react'
import { BUTTON_DANGER, BUTTON_PRIMARY, BUTTON_SECONDARY, RADIUS, SURFACE, TEXT } from '../lib/ui'

interface ConfirmDialogProps {
  title: string
  body: string
  confirmLabel: string
  onConfirm: () => void
  onCancel: () => void
  cancelLabel?: string
  /**
   * `danger` when the action destroys something, `warn` when it is only worth
   * a second look. The icon and the confirming button follow, so the two stops
   * do not look equally final.
   */
  tone?: 'danger' | 'warn'
}

/**
 * A stop before an action that cannot be taken back.
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
  cancelLabel = 'Cancel',
  tone = 'danger',
}: ConfirmDialogProps) {
  const cancelRef = useRef<HTMLButtonElement>(null)
  const destructive = tone === 'danger'
  const Icon = destructive ? Warning : Paperclip

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
          <Icon
            size={20}
            weight="fill"
            aria-hidden
            className={
              destructive
                ? 'mt-0.5 shrink-0 text-[var(--color-danger)] dark:text-[var(--color-danger-dark)]'
                : 'mt-0.5 shrink-0 text-[var(--color-warn)] dark:text-[var(--color-warn-dark)]'
            }
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
            {cancelLabel}
          </button>
          <button
            type="button"
            data-testid="confirm-accept"
            onClick={onConfirm}
            className={`${destructive ? BUTTON_DANGER : BUTTON_PRIMARY} py-1.5`}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
