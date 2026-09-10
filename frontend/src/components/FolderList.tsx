import {
  Archive,
  CircleNotch,
  FileDashed,
  Folder,
  Lock,
  PaperPlaneTilt,
  Trash,
  Tray,
  Warning,
} from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { useMailStore } from '../store/useMailStore'
import { ICON, RADIUS, SELECTED, TEXT } from '../lib/ui'
import type { Folder as MailFolder } from '../lib/api'

/**
 * Folder icons are recognisable at a glance, which is the point: in a list of
 * twenty folders the icon finds Sent faster than the label does.
 */
function iconFor(folder: MailFolder): Icon {
  if (folder.isInbox) return Tray

  const name = folder.name.toLowerCase()
  if (name.includes('sent') || name.includes('gönder')) return PaperPlaneTilt
  if (name.includes('draft') || name.includes('taslak')) return FileDashed
  if (name.includes('trash') || name.includes('çöp') || name.includes('deleted')) return Trash
  if (name.includes('archive') || name.includes('arşiv')) return Archive
  return Folder
}

export function FolderList() {
  const accounts = useMailStore((s) => s.accounts)
  const folders = useMailStore((s) => s.folders)
  const selectedFolderId = useMailStore((s) => s.selectedFolderId)
  const setSelectedFolder = useMailStore((s) => s.setSelectedFolder)
  const syncState = useMailStore((s) => s.syncState)

  return (
    <nav className="p-2" aria-label="Folders">
      {accounts.map((account) => {
        const state = syncState[account.id]

        return (
          <section key={account.id} className="mb-5">
            <header className="flex items-center justify-between gap-2 px-2 pb-1.5">
              <span className={`truncate text-xs font-medium ${TEXT.secondary}`}>
                {account.email}
              </span>

              {state?.status === 'syncing' && (
                <CircleNotch
                  size={ICON.size}
                  weight={ICON.weight}
                  data-testid="sync-indicator"
                  aria-label="Syncing"
                  className={`shrink-0 animate-spin ${TEXT.muted}`}
                />
              )}

              {state?.status === 'error' && (
                <span
                  data-testid="sync-error"
                  data-error-class={state.errorClass}
                  title={state.error}
                  className="shrink-0"
                >
                  {state.errorClass === 'auth' ? (
                    <Lock
                      size={ICON.size}
                      weight={ICON.weight}
                      aria-label="Sign-in required"
                      className="text-[var(--color-warn)] dark:text-[var(--color-warn-dark)]"
                    />
                  ) : (
                    <Warning
                      size={ICON.size}
                      weight={ICON.weight}
                      aria-label="Sync error"
                      className="text-[var(--color-danger)] dark:text-[var(--color-danger-dark)]"
                    />
                  )}
                </span>
              )}
            </header>

            <ul>
              {folders
                .filter((folder) => folder.accountId === account.id)
                .map((folder) => {
                  const FolderIcon = iconFor(folder)
                  const selected = folder.id === selectedFolderId

                  return (
                    <li key={folder.id}>
                      <button
                        type="button"
                        data-testid="folder-row"
                        onClick={() => setSelectedFolder(folder.id)}
                        aria-current={selected}
                        className={[
                          RADIUS,
                          'flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm transition-colors',
                          selected
                            ? `${SELECTED} font-medium ${TEXT.primary}`
                            : `${TEXT.secondary} hover:bg-neutral-200/70 dark:hover:bg-neutral-800`,
                        ].join(' ')}
                      >
                        <FolderIcon
                          size={ICON.size}
                          weight={selected ? 'fill' : ICON.weight}
                          className={
                            selected ? 'shrink-0 text-[var(--color-accent)]' : 'shrink-0'
                          }
                        />
                        <span className="flex-1 truncate">{folder.name}</span>

                        {folder.unreadCount > 0 && (
                          <span className={`tabular shrink-0 font-mono text-xs ${TEXT.muted}`}>
                            {folder.unreadCount}
                          </span>
                        )}
                      </button>
                    </li>
                  )
                })}
            </ul>
          </section>
        )
      })}

      {accounts.length > 0 && folders.length === 0 && (
        <p className={`px-2 py-4 text-xs ${TEXT.muted}`}>
          No folders yet. They appear after the first sync.
        </p>
      )}
    </nav>
  )
}
