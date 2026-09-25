import { Paperclip, PaperPlaneRight, X } from '@phosphor-icons/react'
import { useEffect, useRef, useState } from 'react'
import {
  identities as readIdentities,
  pickAttachments,
  sendMessage,
  type Identity,
  type OutgoingAttachment,
} from '../lib/api'
import { formatSize } from '../lib/format'
import { BUTTON_GHOST, BUTTON_PRIMARY, ICON, ICON_ONLY, RADIUS, SURFACE, TEXT } from '../lib/ui'

/**
 * Writing a message.
 *
 * A full window rather than a dialog, for the same reason Settings is one: a
 * message is written, re-read and rewritten, and a panel that keeps the mail
 * list visible behind it offers a distraction at the one moment the reader is
 * not reading.
 *
 * Plain text only for now, and deliberately so. The rich editor and the
 * recipient pills are worth having and are worth having properly; a textarea
 * that sends a correct message is better than half an editor that sends a
 * malformed one. The backend already accepts HTML when there is HTML to send.
 */
export function Composer({
  accountId,
  onClose,
  reply,
}: {
  accountId: number
  onClose: () => void
  /**
   * Set when answering or forwarding, which carries the threading headers and
   * the quoted original through.
   */
  reply?: {
    subject: string
    to: string
    cc: string
    inReplyTo: string
    references: string[]
    quoted: string
  }
}) {
  const [from, setFrom] = useState<Identity[]>([])
  const [identityId, setIdentityId] = useState(0)

  const [to, setTo] = useState(reply?.to ?? '')
  const [cc, setCc] = useState(reply?.cc ?? '')
  const [bcc, setBcc] = useState('')
  // Cc and Bcc are hidden until asked for. Most messages have neither, and
  // four empty fields at the top of every compose window is four lines of
  // nothing between the writer and the thing they came to write.
  // Shown from the start when a reply-all already put people there: a Cc line
  // with names on it that the writer cannot see is a message going somewhere
  // they did not check.
  const [showCopies, setShowCopies] = useState((reply?.cc ?? '') !== '')

  const [subject, setSubject] = useState(reply?.subject ?? '')
  // The quote starts below an empty line, and the cursor starts above it.
  // Top-posting is what the rest of the world does and what a reader scanning
  // a thread expects; the quote is there to be referred to, not read first.
  const [body, setBody] = useState(reply?.quoted ? `\n\n${reply.quoted}` : '')

  // Files, by path. The bytes stay on disk until Send, so a composer left
  // open with three photographs on it costs this window three filenames.
  const [files, setFiles] = useState<OutgoingAttachment[]>([])

  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const firstField = useRef<HTMLInputElement>(null)
  const bodyField = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    readIdentities(accountId)
      .then((list) => {
        setFrom(list)
        // The list comes back default first, so the first entry is the one to
        // open with.
        if (list.length > 0) setIdentityId(list[0].id)
      })
      .catch((err: unknown) => setError(messageOf(err)))
  }, [accountId])

  // A reply already knows who it is going to, so the cursor belongs in the
  // body; a new message has an empty To line and belongs there.
  useEffect(() => {
    if (reply) {
      // The cursor goes to the top of the body, above the quote.
      bodyField.current?.focus()
      bodyField.current?.setSelectionRange(0, 0)
      return
    }
    firstField.current?.focus()
  }, [reply])

  const canSend = to.trim() !== '' || cc.trim() !== '' || bcc.trim() !== ''

  const attach = async () => {
    try {
      const chosen = await pickAttachments()
      // Appended, and the same file twice is the same file: choosing it again
      // is how somebody ends up mailing two copies of one attachment.
      setFiles((current) => {
        const have = new Set(current.map((f) => f.path))
        return [...current, ...chosen.filter((f) => !have.has(f.path))]
      })
    } catch (err: unknown) {
      setError(messageOf(err))
    }
  }

  const send = async () => {
    if (!canSend || sending) return
    setSending(true)
    setError('')

    try {
      await sendMessage({
        identityId,
        accountId,
        to,
        cc,
        bcc,
        subject,
        text: body,
        html: '',
        inReplyTo: reply?.inReplyTo ?? '',
        references: reply?.references ?? [],
        attachmentPaths: files.map((f) => f.path),
      })
      onClose()
    } catch (err: unknown) {
      // The window stays open with everything in it. A send that failed and a
      // composer that closed would be a message the writer cannot recover.
      setError(messageOf(err))
      setSending(false)
    }
  }

  return (
    <div className={`flex h-full flex-col ${SURFACE.page}`}>
      <header className={`flex items-center gap-3 border-b px-6 py-3 ${SURFACE.divider}`}>
        <h1 className={`flex-1 text-base font-semibold tracking-tight ${TEXT.primary}`}>
          {reply ? (reply.to === '' ? 'Forward' : 'Reply') : 'New message'}
        </h1>

        <button
          type="button"
          data-testid="composer-attach"
          aria-label="Attach files"
          title="Attach files"
          onClick={() => void attach()}
          className={`${BUTTON_GHOST} ${ICON_ONLY}`}
        >
          <Paperclip size={ICON.size} weight={ICON.weight} aria-hidden />
        </button>

        <button
          type="button"
          data-testid="composer-send"
          disabled={!canSend || sending}
          onClick={() => void send()}
          className={`${BUTTON_PRIMARY} inline-flex items-center gap-1.5 py-1.5`}
        >
          <PaperPlaneRight size={ICON.size} weight={ICON.weight} aria-hidden />
          {sending ? 'Sending…' : 'Send'}
        </button>

        <button
          type="button"
          data-testid="composer-close"
          aria-label="Close without sending"
          title="Close without sending"
          onClick={onClose}
          className={`${BUTTON_GHOST} ${ICON_ONLY}`}
        >
          <X size={ICON.size} weight={ICON.weight} aria-hidden />
        </button>
      </header>

      {error && (
        <p
          data-testid="composer-error"
          role="alert"
          className={`border-b px-6 py-2 text-sm ${SURFACE.divider} text-[var(--color-danger)] dark:text-[var(--color-danger-dark)]`}
        >
          {error}
        </p>
      )}

      {/* The address lines are rules rather than boxes. A mail header is a
          list of short labelled values, and a bordered field around each one
          draws five rectangles where the content is four words. */}
      <div className={`divide-y px-6 ${SURFACE.divider}`}>
        {from.length > 1 && (
          <Row label="From" htmlFor="composer-from">
            <select
              id="composer-from"
              data-testid="composer-from"
              value={identityId}
              onChange={(e) => setIdentityId(Number(e.target.value))}
              className={`w-full bg-transparent py-2 text-sm outline-none ${TEXT.primary}`}
            >
              {from.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.from}
                </option>
              ))}
            </select>
          </Row>
        )}

        <Row label="To" htmlFor="composer-to">
          <Line id="composer-to" ref={firstField} value={to} onChange={setTo} />
          {!showCopies && (
            <button
              type="button"
              data-testid="composer-show-copies"
              onClick={() => setShowCopies(true)}
              className={`${BUTTON_GHOST} shrink-0`}
            >
              Cc / Bcc
            </button>
          )}
        </Row>

        {showCopies && (
          <>
            <Row label="Cc" htmlFor="composer-cc">
              <Line id="composer-cc" value={cc} onChange={setCc} />
            </Row>
            <Row label="Bcc" htmlFor="composer-bcc">
              <Line id="composer-bcc" value={bcc} onChange={setBcc} />
            </Row>
          </>
        )}

        <Row label="Subject" htmlFor="composer-subject">
          <Line id="composer-subject" value={subject} onChange={setSubject} />
        </Row>
      </div>

      {files.length > 0 && (
        <ul
          data-testid="composer-attachments"
          className={`flex flex-wrap gap-2 border-b px-6 py-2 ${SURFACE.divider}`}
        >
          {files.map((file) => (
            <li
              key={file.path}
              className={`inline-flex items-center gap-1.5 border py-1 pl-2 pr-1 text-xs ${RADIUS} ${SURFACE.divider} ${TEXT.primary}`}
            >
              <Paperclip size={ICON.size} weight={ICON.weight} aria-hidden />
              <span className="max-w-56 truncate">{file.name}</span>
              <span className={`font-mono ${TEXT.muted}`}>{formatSize(file.size)}</span>
              <button
                type="button"
                aria-label={`Remove ${file.name}`}
                title={`Remove ${file.name}`}
                onClick={() => setFiles((current) => current.filter((f) => f.path !== file.path))}
                className={`${BUTTON_GHOST} p-0.5`}
              >
                <X size={ICON.size} weight={ICON.weight} aria-hidden />
              </button>
            </li>
          ))}
        </ul>
      )}

      <textarea
        ref={bodyField}
        data-testid="composer-body"
        aria-label="Message"
        value={body}
        onChange={(e) => setBody(e.target.value)}
        spellCheck
        className={`min-h-0 flex-1 resize-none bg-transparent px-6 py-4 text-sm leading-relaxed outline-none ${TEXT.primary}`}
      />
    </div>
  )
}

/** One labelled line of the header block. */
function Row({
  label,
  htmlFor,
  children,
}: {
  label: string
  htmlFor: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-center gap-3">
      <label
        htmlFor={htmlFor}
        className={`w-16 shrink-0 text-xs ${TEXT.muted}`}
      >
        {label}
      </label>
      <div className="flex min-w-0 flex-1 items-center gap-2">{children}</div>
    </div>
  )
}

/**
 * An address or subject line.
 *
 * Borderless: the divider above and below is what separates it from its
 * neighbours, and INPUT's own border would draw a second one inside the first.
 */
function Line({
  id,
  value,
  onChange,
  ref,
}: {
  id: string
  value: string
  onChange: (next: string) => void
  ref?: React.Ref<HTMLInputElement>
}) {
  return (
    <input
      id={id}
      data-testid={id}
      ref={ref}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className={`min-w-0 flex-1 bg-transparent py-2 text-sm outline-none ${TEXT.primary} placeholder:text-neutral-500 dark:placeholder:text-neutral-400`}
    />
  )
}

/** Unwraps whatever the bridge threw. */
function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}
