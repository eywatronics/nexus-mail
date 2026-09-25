import { CircleNotch, DownloadSimple, Paperclip } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import { listAttachments, revealAttachment, type Attachment } from '../lib/api'
import { ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'
import { formatSize } from '../lib/format'

/**
 * The files a message carries.
 *
 * The list itself costs no network: the parts were recorded when the header
 * was synced, so it renders with the connection off. Only the bytes need
 * fetching, and only when somebody asks for one — a twenty-megabyte file
 * pulled down with every header sync would undo the point of syncing headers.
 */
export function AttachmentList({ messageId }: { messageId: number }) {
  const [files, setFiles] = useState<Attachment[]>([])
  const [busy, setBusy] = useState<number | null>(null)

  useEffect(() => {
    let current = true
    setFiles([])

    listAttachments(messageId)
      .then((list) => {
        if (current) setFiles(list)
      })
      .catch(() => {})

    // The reader can move on before the list arrives, and writing a previous
    // message's attachments into the pane would be worse than showing none.
    return () => {
      current = false
    }
  }, [messageId])

  if (files.length === 0) return null

  return (
    <div
      data-testid="attachment-list"
      className={`flex flex-wrap items-center gap-1.5 border-b px-6 py-2 ${SURFACE.divider}`}
    >
      <Paperclip
        size={ICON.size}
        weight={ICON.weight}
        aria-hidden
        className={`shrink-0 ${TEXT.muted}`}
      />

      {files.map((file) => (
        <button
          key={file.id}
          type="button"
          data-testid="attachment"
          title={`${file.filename} — ${formatSize(file.size)}`}
          disabled={busy === file.id}
          onClick={() => {
            setBusy(file.id)
            revealAttachment(file.id)
              .then(() => setFiles((was) =>
                was.map((f) => (f.id === file.id ? { ...f, downloaded: true } : f)),
              ))
              .catch(() => {})
              .finally(() => setBusy(null))
          }}
          className={[
            RADIUS,
            'inline-flex max-w-[16rem] items-center gap-1.5 border px-2 py-1 text-xs transition-colors',
            SURFACE.divider,
            TEXT.secondary,
            'hover:bg-neutral-100 active:translate-y-px',
            'disabled:cursor-not-allowed disabled:opacity-50 disabled:active:translate-y-0',
            'dark:hover:bg-neutral-800',
          ].join(' ')}
        >
          {busy === file.id ? (
            <CircleNotch
              size={ICON.size}
              weight={ICON.weight}
              aria-hidden
              className="shrink-0 animate-spin"
            />
          ) : (
            <DownloadSimple size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
          )}
          <span className="truncate">{file.filename || 'attachment'}</span>
          <span className={`tabular shrink-0 font-mono ${TEXT.muted}`}>
            {formatSize(file.size)}
          </span>
        </button>
      ))}
    </div>
  )
}
