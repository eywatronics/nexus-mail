import { Plus } from '@phosphor-icons/react'
import { Events } from '@wailsio/runtime'
import { useCallback, useEffect, useState } from 'react'
import { AddAccount } from './components/AddAccount'
import { FolderList } from './components/FolderList'
import { Layout } from './components/Layout'
import { MessageList } from './components/MessageList'
import { MessageView } from './components/MessageView'
import { ThemeToggle } from './components/ThemeToggle'
import { listAccounts, listFolders, listMessages, openFolder } from './lib/api'
import { EVENTS, type SyncEventPayload } from './lib/events'
import { useMailStore } from './store/useMailStore'
import { BUTTON_GHOST, ICON, SURFACE, TEXT } from './lib/ui'

const PAGE_SIZE = 100

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
      list={<MessageList onLoadMore={loadMore} />}
      reader={<MessageView />}
    />
  )
}
