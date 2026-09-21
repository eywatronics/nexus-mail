import { deleteMessages, markRead, setStarred } from './api'
import { useMailStore } from '../store/useMailStore'

/**
 * The actions the window offers on a message.
 *
 * Each one updates the list immediately and calls the backend, which has
 * already written the change to the local database and queued it for the
 * server by the time it returns. The window is not guessing — it is catching
 * up with a write that already happened.
 *
 * A failure puts the row back the way it was. Leaving it showing the change
 * would be worse than never showing it: the user would believe something was
 * done that was not, and only find out on another device.
 */

/** Marks messages read or unread. */
export async function applyRead(ids: number[], read: boolean): Promise<void> {
  if (ids.length === 0) return

  const store = useMailStore.getState()
  store.applyLocalFlag(ids, 'isRead', read)

  try {
    await markRead(ids, read)
  } catch {
    useMailStore.getState().applyLocalFlag(ids, 'isRead', !read)
  }
}

/** Stars or unstars messages. */
export async function applyStar(ids: number[], starred: boolean): Promise<void> {
  if (ids.length === 0) return

  const store = useMailStore.getState()
  store.applyLocalFlag(ids, 'isStarred', starred)

  try {
    await setStarred(ids, starred)
  } catch {
    useMailStore.getState().applyLocalFlag(ids, 'isStarred', !starred)
  }
}

/**
 * Deletes messages.
 *
 * There is no optimistic revert here. Putting a deleted row back needs the row
 * itself, and the list has already dropped it; the next live sync is what
 * restores an accurate list. A failed delete therefore looks like a message
 * that reappears, which is the honest outcome.
 */
export async function applyDelete(ids: number[]): Promise<void> {
  if (ids.length === 0) return

  // Move on before the rows go, so the reader is left looking at the next
  // message rather than at an empty pane. Here rather than at each call site:
  // the button and the keyboard both did it, and the two could drift.
  useMailStore.getState().selectRelative(1)
  useMailStore.getState().removeLocalMessages(ids)
  await deleteMessages(ids).catch(() => {})
}

/**
 * Deleting, with a stop before the kind that cannot be taken back.
 *
 * Outside the trash, deleting moves the message there and needs no question:
 * the message is still somewhere the reader can find it. Inside the trash it
 * is destroyed on the server, and that is the one place the question earns
 * itself.
 */
export function requestDelete(ids: number[]): void {
  if (ids.length === 0) return

  if (ids.some(isInTrash)) {
    useMailStore.getState().askToConfirmDelete(ids)
    return
  }
  void applyDelete(ids)
}

function isInTrash(id: number): boolean {
  const state = useMailStore.getState()
  const message = state.visibleMessages().find((m) => m.id === id)
  if (!message) return false
  return state.folders.find((f) => f.id === message.folderId)?.role === 'trash'
}

/**
 * The message the actions apply to.
 *
 * One selection for now. Multi-select is a list-widget question rather than an
 * action question, and the action layer already takes a list so it will not
 * need changing when that arrives.
 */
export function selectedIds(): number[] {
  const id = useMailStore.getState().selectedMessageId
  return id === null ? [] : [id]
}

/** The currently selected message, if the loaded list holds it. */
export function selectedMessage() {
  const state = useMailStore.getState()
  return state.visibleMessages().find((m) => m.id === state.selectedMessageId)
}
