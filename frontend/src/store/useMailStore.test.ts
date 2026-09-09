import { beforeEach, describe, expect, it } from 'vitest'
import { EVENTS } from '../lib/events'
import { useMailStore } from './useMailStore'
import type { Message } from '../lib/api'

const message = (id: number): Message => ({
  id,
  folderId: 1,
  uid: id,
  threadId: `<t${id}@x>`,
  subject: `Subject ${id}`,
  fromName: 'Sender',
  fromAddr: 'sender@example.com',
  snippet: 'preview',
  internalDateUnix: 1700000000 + id,
  isRead: false,
  isStarred: false,
  hasAttachments: false,
  bodyFetched: false,
})

beforeEach(() => {
  useMailStore.getState().reset()
})

describe('sync state', () => {
  it('starts with nothing recorded', () => {
    expect(useMailStore.getState().syncState).toEqual({})
  })

  it('marks an account as syncing, then idle', () => {
    const { applySyncEvent } = useMailStore.getState()

    applySyncEvent(EVENTS.syncStarted, { accountId: 1, email: 'a@x' })
    expect(useMailStore.getState().syncState[1]).toEqual({ status: 'syncing' })

    applySyncEvent(EVENTS.syncFinished, { accountId: 1, email: 'a@x' })
    expect(useMailStore.getState().syncState[1]).toEqual({ status: 'idle' })
  })

  // An auth failure needs a sign-in prompt; a transient one needs a spinner.
  // Losing the class would make both look the same to the user.
  it('records the error class on failure', () => {
    useMailStore.getState().applySyncEvent(EVENTS.syncFailed, {
      accountId: 2,
      email: 'b@x',
      error: 'authentication failed',
      class: 'auth',
    })
    expect(useMailStore.getState().syncState[2]).toEqual({
      status: 'error',
      error: 'authentication failed',
      errorClass: 'auth',
    })
  })

  it('keeps per-account state separate', () => {
    const { applySyncEvent } = useMailStore.getState()
    applySyncEvent(EVENTS.syncStarted, { accountId: 1, email: 'a@x' })
    applySyncEvent(EVENTS.syncFailed, {
      accountId: 2,
      email: 'b@x',
      error: 'boom',
      class: 'transient',
    })

    const state = useMailStore.getState().syncState
    expect(state[1].status).toBe('syncing')
    expect(state[2].status).toBe('error')
  })

  it('ignores an event it does not know', () => {
    const before = useMailStore.getState().syncState
    useMailStore.getState().applySyncEvent('something:else', { accountId: 1, email: 'a@x' })
    expect(useMailStore.getState().syncState).toBe(before)
  })
})

describe('folder selection', () => {
  it('clears the selected message when the folder changes', () => {
    useMailStore.setState({ selectedFolderId: 1, selectedMessageId: 42 })
    useMailStore.getState().setSelectedFolder(2)

    const state = useMailStore.getState()
    expect(state.selectedFolderId).toBe(2)
    // Leaving this set would render a message from the previous folder.
    expect(state.selectedMessageId).toBeNull()
  })

  it('resets the list and pagination when the folder changes', () => {
    useMailStore.setState({
      selectedFolderId: 1,
      messages: [message(1), message(2)],
      hasMore: false,
      loadingMessages: true,
    })
    useMailStore.getState().setSelectedFolder(2)

    const state = useMailStore.getState()
    expect(state.messages).toEqual([])
    expect(state.hasMore).toBe(true)
    expect(state.loadingMessages).toBe(false)
  })
})

describe('pagination', () => {
  it('appends pages rather than replacing them', () => {
    const { appendMessages } = useMailStore.getState()

    appendMessages([message(1), message(2)], true)
    appendMessages([message(3)], false)

    const state = useMailStore.getState()
    expect(state.messages.map((m) => m.id)).toEqual([1, 2, 3])
    expect(state.hasMore).toBe(false)
  })

  it('clears the loading flag when a page arrives', () => {
    useMailStore.getState().startMessageLoad()
    expect(useMailStore.getState().loadingMessages).toBe(true)

    useMailStore.getState().appendMessages([message(1)], false)
    expect(useMailStore.getState().loadingMessages).toBe(false)
  })
})
