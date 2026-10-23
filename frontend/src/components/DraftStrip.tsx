import { PencilSimple, X } from '@phosphor-icons/react'
import { useCallback, useEffect, useState } from 'react'
import { discardDraft, drafts as readDrafts, type DraftRecord } from '../lib/api'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_GHOST, ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'

/**
 * The messages this account has started and not sent.
 *
 * Above the Drafts folder rather than inside it, and only there. A draft
 * belongs in Drafts — a second place called something else would be a second
 * Drafts, and nobody would know which one held their message. But these have
 * not reached the server yet, so they are not rows in that folder's list
 * either: they have no UID, no flags and no place in its ordering, and putting
 * them there would mean a row the reader can select but not act on.
 *
 * The strip is the honest shape for that: same folder, visibly not yet part of
 * it. When drafts are uploaded to the Drafts mailbox they will simply appear
 * in the list below and this will stop having anything to show.
 */
export function DraftStrip() {
  const folders = useMailStore((s) => s.folders)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const openComposer = useMailStore((s) => s.openComposer)
  const composing = useMailStore((s) => s.composing)

  const folder = folders.find((f) => f.id === selectedFolderId)
  const showing = folder?.role === 'drafts'
  const accountId = folder?.accountId ?? null

  const [list, setList] = useState<DraftRecord[]>([])

  const refresh = useCallback(() => {
    if (!showing || accountId === null) {
      setList([])
      return
    }
    readDrafts(accountId)
      .then(setList)
      // A strip that cannot be filled shows nothing. This is a convenience
      // above a folder, not the folder itself, and an error banner here would
      // sit over mail the person can still read perfectly well.
      .catch(() => setList([]))
  }, [showing, accountId])

  // Re-read when the composer closes, because that is when a draft was just
  // written or just sent. Watching `composing` rather than polling: the window
  // is the only thing that changes these.
  useEffect(refresh, [refresh, composing])

  if (!showing || list.length === 0) return null

  const discard = async (id: number) => {
    await discardDraft(id)
    refresh()
  }

  return (
    <ul
      data-testid="draft-strip"
      className={`flex flex-wrap gap-2 border-b px-3 py-2 ${SURFACE.divider}`}
    >
      {list.map((draft) => (
        <li
          key={draft.id}
          className={`inline-flex items-center gap-1.5 border py-1 pl-2 pr-1 text-xs ${RADIUS} ${SURFACE.divider}`}
        >
          <button
            type="button"
            data-testid="draft-open"
            onClick={() => openComposer({ accountId: draft.accountId, draft })}
            className={`inline-flex min-w-0 items-center gap-1.5 ${TEXT.primary}`}
          >
            <PencilSimple size={ICON.size} weight={ICON.weight} aria-hidden />
            {/* An untitled draft still has to be clickable, so it gets a word
                rather than an empty button nobody can hit. */}
            <span className="max-w-56 truncate">{draft.subject.trim() || 'No subject'}</span>
          </button>

          <button
            type="button"
            aria-label={`Discard ${draft.subject.trim() || 'this draft'}`}
            title="Discard"
            onClick={() => void discard(draft.id)}
            className={`${BUTTON_GHOST} p-0.5`}
          >
            <X size={ICON.size} weight={ICON.weight} aria-hidden />
          </button>
        </li>
      ))}
    </ul>
  )
}
