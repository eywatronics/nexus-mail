import { useState } from 'react'
import { emptyTrash } from '../lib/api'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST } from '../lib/ui'
import { ConfirmDialog } from './ConfirmDialog'

/**
 * Empties the trash, from inside the trash.
 *
 * Only here. A control that destroyed a folder's contents from anywhere in the
 * window would be one click away from a reader who is not looking at what it
 * would destroy.
 *
 * The count comes from the folder row, which is what the server reported, not
 * from the messages this window has loaded. Those differ — the retention
 * window caps what is kept — and the number somebody is about to act on should
 * be the real one.
 */
export function EmptyTrashButton() {
  const [asking, setAsking] = useState(false)
  const [busy, setBusy] = useState(false)

  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const folder = useMailStore((s) =>
    s.folders.find((f) => f.id === selectedFolderId),
  )

  if (!folder || folder.role !== 'trash') return null

  const count = folder.totalCount

  return (
    <>
      <button
        type="button"
        data-testid="empty-trash"
        disabled={busy || count === 0}
        title={count === 0 ? 'The trash is already empty' : 'Empty the trash'}
        onClick={() => setAsking(true)}
        className={[
          BUTTON_GHOST,
          'shrink-0 whitespace-nowrap px-2 py-1 text-xs',
          'disabled:cursor-not-allowed disabled:opacity-40',
        ].join(' ')}
      >
        Empty trash
      </button>

      {asking && (
        <ConfirmDialog
          title={
            count === 1 ? 'Delete 1 message permanently?' : `Delete ${count} messages permanently?`
          }
          // The sentence names the server and says what this window cannot
          // see. Emptying reaches past the part that has been downloaded, and
          // a reader looking at twenty rows should know the number is not
          // twenty.
          body={
            'Everything in the trash will be removed from the server, including ' +
            'messages this window has not downloaded. This cannot be undone.'
          }
          confirmLabel="Empty trash"
          onCancel={() => setAsking(false)}
          onConfirm={() => {
            setAsking(false)
            setBusy(true)
            emptyTrash(folder.accountId)
              .catch(() => {})
              .finally(() => setBusy(false))
          }}
        />
      )}
    </>
  )
}
