import { Plus } from '@phosphor-icons/react'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'
import { AddAccount } from './components/AddAccount'
import { FolderList } from './components/FolderList'
import { Layout } from './components/Layout'
import { MessageList } from './components/MessageList'
import { MessageView } from './components/MessageView'
import { SearchBox } from './components/SearchBox'
import { ThemeToggle } from './components/ThemeToggle'
import {
  listAccounts,
  listFolders,
  listMessages,
  openFolder,
  searchMessages,
} from './lib/api'
import { EVENTS, type SyncEventPayload } from './lib/events'
import { useMessageShortcuts } from './lib/keyboard'
import { useMailStore } from './store/useMailStore'
import { BUTTON_GHOST, ICON, SURFACE, TEXT } from './lib/ui'

const PAGE_SIZE = 100
const SEARCH_LIMIT = 200

/**
 * Typing is faster than a round trip. Without this, every keystroke starts a
 * query whose results arrive after the next keystroke has already made them
 * wrong, and the list flickers through answers to half-typed words.
 */
const SEARCH_DEBOUNCE_MS = 180

export default function App() {
  const [adding, setAdding] = useState(false)
  const [ready, setReady] = useState(false)

  const accounts = useMailStore((s) => s.accounts)
  const setAccounts = useMailStore((s) => s.setAccounts)
  const setFolders = useMailStore((s) => s.setFolders)
  const startMessageLoad = useMailStore((s) => s.startMessageLoad)
  const appendMessages = useMailStore((s) => s.appendMessages)
  const applySyncEvent = useMailStore((s) => s.applySyncEvent)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const messageCount = useMailStore((s) => s.messages.length)
  const folders = useMailStore((s) => s.folders)
  const searchQuery = useMailStore((s) => s.searchQuery)
  const searching = useMailStore((s) => s.searching)
  const startSearch = useMailStore((s) => s.startSearch)
  const setSearchResults = useMailStore((s) => s.setSearchResults)

  useMessageShortcuts()

  // Search is scoped to the account the user is looking at. Merging every
  // account's results into one list would put work mail in front of someone
  // going through their personal inbox.
  const activeAccountId =
    folders.find((f) => f.id === selectedFolderId)?.accountId ?? accounts[0]?.id ?? null

  const refreshAccounts = useCallback(async () => {
    const list = await listAccounts()
    setAccounts(list)

    const folders = (await Promise.all(list.map((a) => listFolders(a.id)))).flat()
    setFolders(folders)
  }, [setAccounts, setFolders])

  useEffect(() => {
    refreshAccounts()
      .catch(() => {
        // Failures surface through sync events; an empty account list is a
        // legitimate first-run state rather than an error to shout about.
      })
      .finally(() => setReady(true))

    const offs = Object.values(EVENTS).map((name) =>
      Events.On(name, (event: { data: SyncEventPayload }) => {
        applySyncEvent(name, event.data)
        if (name === EVENTS.syncFinished) {
          refreshAccounts().catch(() => {})
        }
      }),
    )
    return () => offs.forEach((off) => off())
  }, [applySyncEvent, refreshAccounts])

  // Load the first page whenever the selected folder changes. OpenFolder also
  // syncs a folder the initial sync deliberately left empty.
  useEffect(() => {
    if (selectedFolderId === null) return

    startMessageLoad()
    openFolder(selectedFolderId, PAGE_SIZE)
      .then((page) => appendMessages(page, page.length === PAGE_SIZE))
      .catch(() => appendMessages([], false))
  }, [selectedFolderId, startMessageLoad, appendMessages])

  useEffect(() => {
    if (!searching || activeAccountId === null) return

    startSearch()
    const timer = setTimeout(() => {
      searchMessages(activeAccountId, searchQuery, SEARCH_LIMIT)
        .then(setSearchResults)
        .catch(() => setSearchResults([]))
    }, SEARCH_DEBOUNCE_MS)

    return () => clearTimeout(timer)
  }, [searching, searchQuery, activeAccountId, startSearch, setSearchResults])

  const loadMore = useCallback(() => {
    if (selectedFolderId === null) return

    startMessageLoad()
    listMessages(selectedFolderId, PAGE_SIZE, messageCount)
      .then((page) => appendMessages(page, page.length === PAGE_SIZE))
      .catch(() => appendMessages([], false))
  }, [selectedFolderId, messageCount, startMessageLoad, appendMessages])

  if (!ready) {
    return (
      <div className={`flex h-screen items-center justify-center ${SURFACE.page}`}>
        <p className={`text-sm ${TEXT.secondary}`}>Starting…</p>
      </div>
    )
  }

  if (adding || accounts.length === 0) {
    return (
      <AddAccount
        onDone={() => {
          setAdding(false)
          refreshAccounts().catch(() => {})
        }}
        onCancel={accounts.length > 0 ? () => setAdding(false) : undefined}
      />
    )
  }

  return (
    <Layout
      sidebar={
        <div className="flex h-full flex-col">
          <div
            className={`flex items-center justify-between gap-2 border-b p-2 ${SURFACE.divider}`}
          >
            <button
              type="button"
              onClick={() => setAdding(true)}
              className={`${BUTTON_GHOST} inline-flex items-center gap-1.5`}
            >
              <Plus size={ICON.size} weight={ICON.weight} aria-hidden />
              Account
            </button>
            <ThemeToggle />
          </div>
          <div className="flex-1 overflow-y-auto">
            <FolderList />
          </div>
        </div>
      }
      list={
        <div className="flex h-full flex-col">
          <SearchBox />
          <div className="min-h-0 flex-1">
            <MessageList onLoadMore={loadMore} />
          </div>
        </div>
      }
      reader={<MessageView />}
    />
  )
}
