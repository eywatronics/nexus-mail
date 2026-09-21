import { applyDelete } from '../lib/actions'
import { useMailStore } from '../store/useMailStore'
import { ConfirmDialog } from './ConfirmDialog'

/**
 * The stop before mail is destroyed.
 *
 * Only ever reached from inside the trash. Everywhere else deleting moves the
 * message there, which needs no question — the message is still somewhere the
 * reader can find it.
 *
 * The wording names the server on purpose. "Delete" is a word this reader has
 * pressed a hundred times without consequence, and the thing that makes this
 * press different is that it reaches past the local copy.
 */
export function ConfirmDeleteDialog() {
  const pending = useMailStore((s) => s.pendingDelete)
  const cancel = useMailStore((s) => s.cancelPendingDelete)

  if (pending === null || pending.length === 0) return null

  const many = pending.length > 1

  return (
    <ConfirmDialog
      title={many ? `Delete ${pending.length} messages permanently?` : 'Delete permanently?'}
      body={
        many
          ? 'These messages will be removed from the server. This cannot be undone.'
          : 'This message will be removed from the server. This cannot be undone.'
      }
      confirmLabel="Delete"
      onCancel={cancel}
      onConfirm={() => {
        cancel()
        void applyDelete(pending)
      }}
    />
  )
}
