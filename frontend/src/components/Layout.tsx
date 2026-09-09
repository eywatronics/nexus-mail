import { Group, Panel, Separator, useDefaultLayout } from 'react-resizable-panels'
import type { ReactNode } from 'react'

interface LayoutProps {
  sidebar: ReactNode
  list: ReactNode
  reader: ReactNode
}

const GROUP_ID = 'nexus-mail-columns'
const PANEL_IDS = ['folders', 'messages', 'reader']

/**
 * Three resizable columns: folders, message list, message body.
 *
 * The layout is saved to localStorage, so an arrangement the user sets up
 * survives a restart. A mail client is somewhere people spend hours; making
 * them re-drag the columns every morning would be its own small insult.
 */
export function Layout({ sidebar, list, reader }: LayoutProps) {
  const layout = useDefaultLayout({
    id: GROUP_ID,
    panelIds: PANEL_IDS,
    storage: typeof localStorage === 'undefined' ? undefined : localStorage,
  })

  const separator =
    'w-px shrink-0 cursor-col-resize bg-neutral-200 transition-colors hover:bg-blue-500 dark:bg-neutral-800'

  return (
    <Group
      id={GROUP_ID}
      orientation="horizontal"
      className="flex h-screen"
      {...layout}
    >
      <Panel id="folders" defaultSize="18%" minSize="12%" maxSize="32%">
        <aside
          aria-label="Folders"
          className="h-full overflow-hidden border-r border-neutral-200 bg-neutral-50 dark:border-neutral-800 dark:bg-neutral-900"
        >
          {sidebar}
        </aside>
      </Panel>

      <Separator className={separator} />

      <Panel id="messages" defaultSize="32%" minSize="22%">
        <section
          aria-label="Messages"
          className="h-full overflow-hidden border-r border-neutral-200 dark:border-neutral-800"
        >
          {list}
        </section>
      </Panel>

      <Separator className={separator} />

      <Panel id="reader" defaultSize="50%" minSize="30%">
        <main aria-label="Message" className="h-full overflow-hidden bg-white dark:bg-neutral-950">
          {reader}
        </main>
      </Panel>
    </Group>
  )
}
