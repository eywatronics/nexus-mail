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

describe('search', () => {
  it('is off until a query is set', () => {
    expect(useMailStore.getState().searching).toBe(false)
    expect(useMailStore.getState().visibleMessages()).toEqual([])
  })

  // The list has to switch sources the moment a query exists, otherwise the
  // folder's messages stay on screen under a search box that appears to have
  // done nothing.
  it('shows results instead of the folder once a query is set', () => {
    const { setSelectedFolder, appendMessages, setSearchQuery, setSearchResults } =
      useMailStore.getState()

    setSelectedFolder(1)
    appendMessages([message(1), message(2)], false)
    expect(useMailStore.getState().visibleMessages()).toHaveLength(2)

    setSearchQuery('rapor')
    setSearchResults([message(9)])

    expect(useMailStore.getState().searching).toBe(true)
    expect(useMailStore.getState().visibleMessages()).toEqual([message(9)])
  })

  // Whitespace is not a search. Treating it as one puts the list into an
  // empty-results state that looks like a mailbox with nothing in it.
  it('does not treat whitespace as a query', () => {
    useMailStore.getState().setSearchQuery('   ')
    expect(useMailStore.getState().searching).toBe(false)
  })

  it('restores the folder list when the search is cleared', () => {
    const { setSelectedFolder, appendMessages, setSearchQuery, setSearchResults, clearSearch } =
      useMailStore.getState()

    setSelectedFolder(1)
    appendMessages([message(1), message(2)], false)
    setSearchQuery('rapor')
    setSearchResults([message(9)])

    clearSearch()

    expect(useMailStore.getState().searchQuery).toBe('')
    expect(useMailStore.getState().visibleMessages()).toHaveLength(2)
  })

  // A result carries a selection that belongs to a message the folder list may
  // not contain. Keeping it would leave the reader showing a message that is
  // no longer anywhere on screen.
  it('drops a selection that came from the results', () => {
    const { setSelectedFolder, appendMessages, setSearchQuery, setSearchResults, clearSearch } =
      useMailStore.getState()

    setSelectedFolder(1)
    appendMessages([message(1)], false)
    setSearchQuery('rapor')
    setSearchResults([message(9)])
    useMailStore.getState().setSelectedMessage(9)

    clearSearch()

    expect(useMailStore.getState().selectedMessageId).toBeNull()
  })

  it('keeps a selection the folder list still holds', () => {
    const { setSelectedFolder, appendMessages, setSearchQuery, setSearchResults, clearSearch } =
      useMailStore.getState()

    setSelectedFolder(1)
    appendMessages([message(1), message(2)], false)
    setSearchQuery('rapor')
    setSearchResults([message(1)])
    useMailStore.getState().setSelectedMessage(1)

    clearSearch()

    expect(useMailStore.getState().selectedMessageId).toBe(1)
  })

  it('changing folder ends the search', () => {
    const { setSearchQuery, setSearchResults, setSelectedFolder } = useMailStore.getState()

    setSearchQuery('rapor')
    setSearchResults([message(9)])
    setSelectedFolder(2)

    expect(useMailStore.getState().searching).toBe(false)
  })
})

describe('keyboard selection', () => {
  const seed = (count: number) => {
    useMailStore.getState().setSelectedFolder(1)
    useMailStore
      .getState()
      .appendMessages(Array.from({ length: count }, (_, i) => message(i + 1)), false)
  }

  it('selects the first message when nothing is selected', () => {
    seed(3)
    useMailStore.getState().selectRelative(1)
    expect(useMailStore.getState().selectedMessageId).toBe(1)
  })

  it('moves down and up through the list', () => {
    seed(3)
    const { selectRelative } = useMailStore.getState()

    selectRelative(1)
    selectRelative(1)
    expect(useMailStore.getState().selectedMessageId).toBe(2)

    selectRelative(-1)
    expect(useMailStore.getState().selectedMessageId).toBe(1)
  })

  // Wrapping around would move the reader to the opposite end of a fifty
  // thousand message list on one keystroke too many.
  it('stops at both ends rather than wrapping', () => {
    seed(2)
    const { selectRelative } = useMailStore.getState()

    selectRelative(-1)
    expect(useMailStore.getState().selectedMessageId).toBe(1)

    selectRelative(1)
    selectRelative(1)
    selectRelative(1)
    expect(useMailStore.getState().selectedMessageId).toBe(2)
  })

  it('walks the results while a search is open', () => {
    seed(3)
    const { setSearchQuery, setSearchResults, selectRelative } = useMailStore.getState()

    setSearchQuery('rapor')
    setSearchResults([message(41), message(42)])

    selectRelative(1)
    expect(useMailStore.getState().selectedMessageId).toBe(41)
    selectRelative(1)
    expect(useMailStore.getState().selectedMessageId).toBe(42)
  })

  it('does nothing when there is nothing to select', () => {
    useMailStore.getState().selectRelative(1)
    expect(useMailStore.getState().selectedMessageId).toBeNull()
  })
})
