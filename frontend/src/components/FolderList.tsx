import { useMailStore } from '../store/useMailStore'

export function FolderList() {
  const accounts = useMailStore((s) => s.accounts)
  const folders = useMailStore((s) => s.folders)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const setSelectedFolder = useMailStore((s) => s.setSelectedFolder)
  const syncState = useMailStore((s) => s.syncState)

  return (
    <nav className="p-2 text-sm" aria-label="Folders">
      {accounts.map((account) => {
        const state = syncState[account.id]
        return (
          <div key={account.id} className="mb-4">
            <div className="flex items-center justify-between gap-2 px-2 py-1">
              <span className="truncate font-medium text-neutral-700 dark:text-neutral-300">
                {account.email}
              </span>

              {state?.status === 'syncing' && (
                <span
                  data-testid="sync-indicator"
                  aria-label="Syncing"
                  className="shrink-0 text-xs text-neutral-500"
                >
                  ↻
                </span>
              )}

              {state?.status === 'error' && (
                <span
                  data-testid="sync-error"
                  data-error-class={state.errorClass}
                  title={state.error}
                  aria-label={
                    state.errorClass === 'auth' ? 'Sign-in required' : 'Sync error'
                  }
                  className="shrink-0 text-xs text-red-600"
                >
                  {state.errorClass === 'auth' ? '🔒' : '!'}
                </span>
              )}
            </div>

            <ul>
              {folders
                .filter((folder) => folder.accountId === account.id)
                .map((folder) => (
                  <li key={folder.id}>
                    <button
                      type="button"
                      data-testid="folder-row"
                      onClick={() => setSelectedFolder(folder.id)}
                      aria-current={folder.id === selectedFolderId}
                      className={`flex w-full items-center justify-between gap-2 rounded px-2 py-1 text-left hover:bg-neutral-200 dark:hover:bg-neutral-800 ${
                        folder.id === selectedFolderId
                          ? 'bg-neutral-200 font-medium dark:bg-neutral-800'
                          : ''
                      }`}
                    >
                      <span className="truncate">{folder.name}</span>
                      {folder.unreadCount > 0 && (
                        <span className="shrink-0 text-xs text-neutral-500">
                          {folder.unreadCount}
                        </span>
                      )}
                    </button>
                  </li>
                ))}
            </ul>
          </div>
        )
      })}
    </nav>
  )
}
