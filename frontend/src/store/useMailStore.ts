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

  setAccounts: (accounts: Account[]) => void
  setFolders: (folders: Folder[]) => void
  startMessageLoad: () => void
  appendMessages: (messages: Message[], hasMore: boolean) => void
  setSelectedFolder: (id: number | null) => void
  setSelectedMessage: (id: number | null) => void
  applySyncEvent: (name: string, payload: SyncEventPayload) => void
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
}

export const useMailStore = create<MailState>((set) => ({
  ...initialState,

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

  reset: () => set(initialState),
}))
