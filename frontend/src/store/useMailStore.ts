import { create } from 'zustand'
import { EVENTS, type ErrorClass, type SyncEventPayload } from '../lib/events'
import type { Account, Folder, Message } from '../lib/api'

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

  setAccounts: (accounts: Account[]) => void
  setFolders: (folders: Folder[]) => void
  startMessageLoad: () => void
  appendMessages: (messages: Message[], hasMore: boolean) => void
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
}

export const useMailStore = create<MailState>((set, get) => ({
  ...initialState,

  visibleMessages: () => {
    const state = get()
    return state.searching ? state.searchResults : state.messages
  },

  setAccounts: (accounts) => set({ accounts }),
  setFolders: (folders) => set({ folders }),

  startMessageLoad: () => set({ loadingMessages: true }),

  appendMessages: (messages, hasMore) =>
    set((state) => ({
      messages: [...state.messages, ...messages],
      hasMore,
      loadingMessages: false,
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
