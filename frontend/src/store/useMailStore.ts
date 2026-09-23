import { create } from 'zustand'
import { EVENTS, type ErrorClass, type SyncEventPayload } from '../lib/events'
import { readPref, writePref } from '../lib/prefs'
import { THEME_CHOICES, type ThemeChoice } from '../lib/theme'
import type { Account, Folder, Message, PendingChanges, Undoable } from '../lib/api'

export type SyncStatus =
  | { status: 'idle' }
  | { status: 'syncing' }
  | { status: 'error'; error: string; errorClass?: ErrorClass }

interface MailState {
  accounts: Account[]
  folders: Folder[]
  messages: Message[]
  selectedFolderId: number | null
  selectedMessageId: number | null
  hasMore: boolean
  loadingMessages: boolean
  syncState: Record<number, SyncStatus>

  /**
   * Search is a second source for the same column rather than a filter over
   * the first. Results span folders, so they cannot be expressed as a subset
   * of the messages a folder happens to have loaded.
   */
  searchQuery: string
  searchResults: Message[]
  searchPending: boolean

  /** Per account: how much of the user's intent has not reached the server. */
  pendingChanges: Record<number, PendingChanges>

  /**
   * Whether the list groups messages into conversations.
   *
   * Held here rather than in the list component because it changes what the
   * backend is asked for, not just how the result is drawn: the threaded order
   * comes from the database. Remembered across restarts, like the theme.
   */
  threaded: boolean
  setThreaded: (next: boolean) => void

  /**
   * Which theme the reader picked: light, dark, or follow the machine.
   *
   * Here rather than inside the theme control because two places need it. The
   * reading pane is a sandboxed frame that cannot see the class on the host
   * page, so the theme travels to the backend in the body URL; a second copy
   * of the answer would drift from this one.
   */
  themeChoice: ThemeChoice
  setThemeChoice: (next: ThemeChoice) => void

  /**
   * Messages waiting on a "yes, destroy these" before they are destroyed.
   *
   * In the store rather than in a component because two things ask for it —
   * the toolbar button and the Delete key — and a second dialog mounted by the
   * keyboard path would be a second dialog to keep in step with the first.
   */
  pendingDelete: number[] | null
  askToConfirmDelete: (ids: number[]) => void
  cancelPendingDelete: () => void

  /**
   * The last destructive action, while it can still be taken back.
   *
   * Null once the window has closed, so the offer disappears rather than
   * leaving a button that would fail — a button that sometimes silently does
   * nothing is worse than no button.
   */
  undoOffer: Undoable | null
  setUndoOffer: (offer: Undoable | null) => void

  /**
   * What redo would do again, while it is still on offer.
   *
   * A second field rather than a direction on the first, because the two are
   * never live together: taking an undo is what creates a redo, and doing
   * anything at all ends it. Folding them into one would mean a component
   * reading a flag to know which of two sentences to show, for no case where
   * both exist.
   */
  redoOffer: Undoable | null
  setRedoOffer: (offer: Undoable | null) => void

  /**
   * Whether the reading pane's find bar is showing.
   *
   * Only the flag is here. Ctrl+F comes from the global key handler and Escape
   * has to close the bar from anywhere, including the message list, so the two
   * ends of it are in different components; what is being searched for stays
   * with the pane that is searching.
   */
  findOpen: boolean
  openFind: () => void
  closeFind: () => void

  setAccounts: (accounts: Account[]) => void
  setPendingChanges: (accountId: number, counts: PendingChanges) => void
  clearPendingChanges: (accountId: number) => void
  setFolders: (folders: Folder[]) => void
  startMessageLoad: () => void
  appendMessages: (messages: Message[], hasMore: boolean) => void
  /** Swaps the folder's list for a freshly fetched one, after a live sync. */
  replaceMessages: (messages: Message[], hasMore: boolean) => void

  /**
   * Reflects a change the backend has already written, before any round trip.
   * The database is the source of truth; this is the window catching up in the
   * same frame the user clicked.
   */
  applyLocalFlag: (ids: number[], field: 'isRead' | 'isStarred', value: boolean) => void
  /** Drops messages the user deleted or moved out of the current folder. */
  removeLocalMessages: (ids: number[]) => void
  setSelectedFolder: (id: number | null) => void
  setSelectedMessage: (id: number | null) => void
  applySyncEvent: (name: string, payload: SyncEventPayload) => void

  setSearchQuery: (query: string) => void
  setSearchResults: (messages: Message[]) => void
  startSearch: () => void
  clearSearch: () => void

  /** True while a query is worth running: whitespace is not a search. */
  searching: boolean
  /** The messages the list column is showing right now, from either source. */
  visibleMessages: () => Message[]
  /** Moves the selection by one row through whatever is on screen. */
  selectRelative: (delta: number) => void

  reset: () => void
}

const THREADED_KEY = 'nexus-mail-threaded'
const THREADED_VALUES = ['on', 'off'] as const
const THEME_KEY = 'nexus-mail-theme'

const initialState = {
  accounts: [] as Account[],
  folders: [] as Folder[],
  messages: [] as Message[],
  selectedFolderId: null as number | null,
  selectedMessageId: null as number | null,
  hasMore: true,
  loadingMessages: false,
  syncState: {} as Record<number, SyncStatus>,
  searchQuery: '',
  searchResults: [] as Message[],
  searchPending: false,
  searching: false,
  pendingChanges: {} as Record<number, PendingChanges>,
  pendingDelete: null as number[] | null,
  undoOffer: null as Undoable | null,
  redoOffer: null as Undoable | null,
  findOpen: false,
  threaded: readPref(THREADED_KEY, THREADED_VALUES, 'off') === 'on',
  themeChoice: readPref<ThemeChoice>(THEME_KEY, THEME_CHOICES, 'system'),
}

export const useMailStore = create<MailState>((set, get) => ({
  ...initialState,

  openFind: () => set({ findOpen: true }),
  closeFind: () => set({ findOpen: false }),

  askToConfirmDelete: (ids) => set({ pendingDelete: ids }),
  setUndoOffer: (offer) => set({ undoOffer: offer }),
  setRedoOffer: (offer) => set({ redoOffer: offer }),
  cancelPendingDelete: () => set({ pendingDelete: null }),

  setThemeChoice: (next) => {
    writePref(THEME_KEY, next)
    set({ themeChoice: next })
  },

  setThreaded: (next) => {
    writePref(THREADED_KEY, next ? 'on' : 'off')
    // The order is different, so the loaded page is no longer the right page.
    // Clearing it rather than re-sorting locally: the backend ranks threads by
    // their newest message across the whole folder, which a page cannot.
    set({ threaded: next, messages: [], hasMore: true })
  },

  visibleMessages: () => {
    const state = get()
    return state.searching ? state.searchResults : state.messages
  },

  setAccounts: (accounts) => set({ accounts }),

  setPendingChanges: (accountId, counts) =>
    set((state) => ({ pendingChanges: { ...state.pendingChanges, [accountId]: counts } })),

  clearPendingChanges: (accountId) =>
    set((state) => ({
      pendingChanges: { ...state.pendingChanges, [accountId]: { pending: 0, dropped: 0, failed: 0 } },
    })),
  setFolders: (folders) => set({ folders }),

  startMessageLoad: () => set({ loadingMessages: true }),

  appendMessages: (messages, hasMore) =>
    set((state) => ({
      messages: [...state.messages, ...messages],
      hasMore,
      loadingMessages: false,
    })),

  replaceMessages: (messages, hasMore) =>
    set((state) => ({
      messages,
      hasMore,
      loadingMessages: false,
      // A message expunged on another device is gone from the refresh.
      // Keeping it selected would leave the reader asking the server for mail
      // that no longer exists.
      selectedMessageId: messages.some((m) => m.id === state.selectedMessageId)
        ? state.selectedMessageId
        : null,
    })),

  // Changing folder clears the list, the selection and pagination together.
  // Keeping any one of them would render or fetch content belonging to the
  // folder the user just left.
  setSelectedFolder: (id) =>
    set({
      selectedFolderId: id,
      selectedMessageId: null,
      messages: [],
      hasMore: true,
      loadingMessages: false,
      // Picking a folder is a statement about what to look at, so it ends the
      // search. Leaving the results up would show the folder as selected while
      // the column beside it still listed something else.
      searchQuery: '',
      searchResults: [],
      searchPending: false,
      searching: false,
    }),

  setSelectedMessage: (id) => set({ selectedMessageId: id }),

  // Both lists are updated, because a search result and a folder row are two
  // views of the same message. Starring one and not the other would read as a
  // bug the moment the user closed the search.
  applyLocalFlag: (ids, field, value) =>
    set((state) => {
      const touched = new Set(ids)
      const patch = (list: Message[]) =>
        list.map((m) => (touched.has(m.id) ? { ...m, [field]: value } : m))

      return { messages: patch(state.messages), searchResults: patch(state.searchResults) }
    }),

  removeLocalMessages: (ids) =>
    set((state) => {
      const gone = new Set(ids)
      const keep = (list: Message[]) => list.filter((m) => !gone.has(m.id))

      return {
        messages: keep(state.messages),
        searchResults: keep(state.searchResults),
        // Leaving a deleted message selected would keep the reading pane
        // asking the backend for mail that is no longer there.
        selectedMessageId: gone.has(state.selectedMessageId ?? -1)
          ? null
          : state.selectedMessageId,
      }
    }),

  applySyncEvent: (name, payload) =>
    set((state) => {
      let next: SyncStatus
      switch (name) {
        case EVENTS.syncStarted:
          next = { status: 'syncing' }
          break
        case EVENTS.syncFinished:
          next = { status: 'idle' }
          break
        case EVENTS.syncFailed:
          next = {
            status: 'error',
            error: payload.error ?? 'unknown error',
            errorClass: payload.class,
          }
          break
        default:
          return state
      }
      return { syncState: { ...state.syncState, [payload.accountId]: next } }
    }),

  setSearchQuery: (query) =>
    set({
      searchQuery: query,
      searching: query.trim() !== '',
      // Results from the previous query are dropped immediately. Leaving them
      // under a changed box shows answers to a question no longer being asked.
      searchResults: [],
    }),

  setSearchResults: (messages) => set({ searchResults: messages, searchPending: false }),

  startSearch: () => set({ searchPending: true }),

  clearSearch: () =>
    set((state) => ({
      searchQuery: '',
      searchResults: [],
      searchPending: false,
      searching: false,
      // A result may be from a folder the list has not loaded. Keeping that
      // selection would leave the reader on a message that is no longer
      // anywhere on screen.
      selectedMessageId: state.messages.some((m) => m.id === state.selectedMessageId)
        ? state.selectedMessageId
        : null,
    })),

  selectRelative: (delta) =>
    set((state) => {
      const list = state.searching ? state.searchResults : state.messages
      if (list.length === 0) return state

      const current = list.findIndex((m) => m.id === state.selectedMessageId)
      // From no selection, either direction lands on the first row: the reader
      // is at the top of the list, not off the end of it.
      const next = current === -1 ? 0 : Math.min(Math.max(current + delta, 0), list.length - 1)

      return { selectedMessageId: list[next].id }
    }),

  reset: () => set(initialState),
}))
