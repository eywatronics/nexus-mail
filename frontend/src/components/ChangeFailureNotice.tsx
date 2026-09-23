import { Warning, X } from '@phosphor-icons/react'
import { acknowledgeChangeFailures } from '../lib/api'
import { useMailStore } from '../store/useMailStore'
import { ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'

/**
 * Tells the user about changes that will never reach the server.
 *
 * This is the visible half of the UIDVALIDITY rule. When the server recreates
 * a mailbox, every queued change against it names UIDs that now belong to
 * different messages, so the worker drops them rather than acting on the wrong
 * mail. That is the right trade — but only if it is said out loud. Dropping
 * them silently would turn a loss of the user's intent into something they
 * never find out about, which is the failure the whole mechanism exists to
 * avoid.
 */
export function ChangeFailureNotice() {
  const accounts = useMailStore((s) => s.accounts)
  const pending = useMailStore((s) => s.pendingChanges)
  const clearPendingChanges = useMailStore((s) => s.clearPendingChanges)

  const troubled = accounts.filter((a) => {
    const counts = pending[a.id]
    return counts && counts.dropped + counts.failed > 0
  })
  if (troubled.length === 0) return null

  return (
    <div className={`flex flex-col gap-2 border-b p-2 ${SURFACE.divider}`}>
      {troubled.map((account) => {
        const counts = pending[account.id]
        const total = counts.dropped + counts.failed

        return (
          <div
            key={account.id}
            role="alert"
            data-testid="change-failure"
            className={`flex items-start gap-2 border p-2.5 ${RADIUS} border-[var(--color-warn)]/30`}
          >
            <Warning
              size={ICON.size}
              weight="fill"
              aria-hidden
              className="mt-0.5 shrink-0 text-[var(--color-warn)] dark:text-[var(--color-warn-dark)]"
            />

            <div className="min-w-0 flex-1">
              <p className={`text-xs leading-relaxed ${TEXT.primary}`}>
                {total === 1
                  ? '1 change could not be applied'
                  : `${total} changes could not be applied`}
                {accounts.length > 1 && ` in ${account.email}`}.
              </p>
              {counts.dropped > 0 && (
                <p className={`mt-0.5 text-xs leading-relaxed ${TEXT.secondary}`}>
                  The server rebuilt the folder they belonged to, so the messages
                  they pointed at are no longer the same ones. They were not
                  applied to anything else.
                </p>
              )}
            </div>

            <button
              type="button"
              data-testid="dismiss-change-failure"
              aria-label="Dismiss"
              onClick={() => {
                // Cleared in the database rather than hidden here: the count is
                // what the notice is made of, so the two cannot disagree about
                // whether the user has been told.
                void acknowledgeChangeFailures(account.id)
                  .then(() => clearPendingChanges(account.id))
                  .catch(() => {})
              }}
              className={`shrink-0 rounded-[var(--radius-ui)] p-1 transition-colors hover:bg-neutral-200 dark:hover:bg-neutral-800 ${TEXT.muted}`}
            >
              <X size={ICON.size} weight={ICON.weight} aria-hidden />
            </button>
          </div>
        )
      })}
    </div>
  )
}
