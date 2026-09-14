import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useMailStore } from '../store/useMailStore'
import { MessageView } from './MessageView'

beforeEach(() => {
  useMailStore.getState().reset()
})

async function renderSelected(id: number) {
  useMailStore.setState({ selectedMessageId: id })
  const view = render(<MessageView />)
  const iframe = await waitFor(() => {
    const el = view.container.querySelector('iframe')
    expect(el).toBeTruthy()
    return el as HTMLIFrameElement
  })
  return { ...view, iframe }
}

describe('isolation', () => {
  it('renders the body inside a sandboxed frame', async () => {
    const { iframe } = await renderSelected(1)

    const sandbox = iframe.getAttribute('sandbox')
    expect(sandbox).not.toBeNull()

    // Granting allow-same-origin would give mail scripts access to this app's
    // origin, including local storage and the Wails bridge. Its absence is the
    // single most important assertion in the frontend.
    expect(sandbox).not.toContain('allow-same-origin')
    expect(sandbox).not.toContain('allow-scripts')
  })

  it('points the frame at the body endpoint rather than inlining content', async () => {
    const { iframe } = await renderSelected(7)

    // Bodies travel over HTTP, not the bridge: an eight-megabyte newsletter
    // serialised into an IPC message would visibly block the UI thread.
    expect(iframe.getAttribute('src')).toBe('/mail-body/7')
    expect(iframe.getAttribute('srcdoc')).toBeNull()
  })
})

describe('remote content', () => {
  it('blocks remote content until asked', async () => {
    const { getByTestId, iframe } = await renderSelected(1)

    expect(getByTestId('load-remote')).toBeTruthy()
    expect(iframe.getAttribute('src')).not.toContain('remote=1')
  })

  it('reloads through the proxy once consent is given', async () => {
    const { getByTestId, container } = await renderSelected(1)

    getByTestId('load-remote').click()

    await waitFor(() => {
      const iframe = container.querySelector('iframe') as HTMLIFrameElement
      expect(iframe.getAttribute('src')).toBe('/mail-body/1?remote=1')
    })
  })

  it('forgets consent when a different message is opened', async () => {
    const { getByTestId, container } = await renderSelected(1)
    getByTestId('load-remote').click()

    await waitFor(() => {
      const iframe = container.querySelector('iframe') as HTMLIFrameElement
      expect(iframe.getAttribute('src')).toContain('remote=1')
    })

    // Carrying consent forward would silently load trackers in whatever the
    // user opens next.
    useMailStore.setState({ selectedMessageId: 2 })

    await waitFor(() => {
      const iframe = container.querySelector('iframe') as HTMLIFrameElement
      expect(iframe.getAttribute('src')).toBe('/mail-body/2')
    })
  })
})

describe('placeholder', () => {
  it('shows a prompt when no message is selected', () => {
    useMailStore.setState({ selectedMessageId: null })
    const { container } = render(<MessageView />)

    expect(container.querySelector('iframe')).toBeNull()
    expect(container.textContent).toMatch(/select a message/i)
  })
})

describe('message header', () => {
  // Once the reader is in the body the list may be scrolled far away. Without
  // a header there is nothing on screen saying who wrote this or what it is
  // about.
  it('shows subject, sender and date above the body', async () => {
    useMailStore.setState({
      messages: [
        {
          id: 1,
          folderId: 1,
          uid: 1,
          threadId: '<t1@x>',
          subject: 'Şubat mutabakat dosyası',
          fromName: 'Zeynep Aydoğan',
          fromAddr: 'zeynep.aydogan@example.com',
          snippet: 'preview',
          internalDateUnix: 1700000000,
          isRead: true,
          isStarred: false,
          hasAttachments: false,
          bodyFetched: false,
        },
      ],
    })

    const { container } = await renderSelected(1)
    const header = container.querySelector('header')

    expect(header).toBeTruthy()
    expect(header?.textContent).toContain('Şubat mutabakat dosyası')
    expect(header?.textContent).toContain('Zeynep Aydoğan')
    expect(header?.textContent).toContain('zeynep.aydogan@example.com')
  })

  // A message the list has not loaded still has to render its body rather than
  // crashing on the missing header data.
  it('renders the body even when the message is not in the loaded page', async () => {
    useMailStore.setState({ messages: [] })

    const { container, iframe } = await renderSelected(42)

    expect(container.querySelector('header')).toBeNull()
    expect(iframe.getAttribute('src')).toBe('/mail-body/42')
  })
})

describe('message actions', () => {
  const seedSelected = (overrides: Partial<import('../lib/api').Message> = {}) =>
    useMailStore.setState({
      messages: [
        {
          id: 1,
          folderId: 1,
          uid: 1,
          threadId: '<t1@x>',
          subject: 'Konu',
          fromName: 'Gönderen',
          fromAddr: 'g@example.com',
          snippet: 'önizleme',
          internalDateUnix: 1700000000,
          isRead: true,
          isStarred: false,
          hasAttachments: false,
          bodyFetched: false,
          ...overrides,
        },
      ],
    })

  it('toggles read from the header', async () => {
    seedSelected({ isRead: true })
    const { getByTestId } = await renderSelected(1)

    getByTestId('toggle-read').click()

    await waitFor(() => {
      expect(useMailStore.getState().messages[0].isRead).toBe(false)
    })
  })

  it('toggles the star from the header', async () => {
    seedSelected({ isStarred: false })
    const { getByTestId } = await renderSelected(1)

    getByTestId('toggle-star').click()

    await waitFor(() => {
      expect(useMailStore.getState().messages[0].isStarred).toBe(true)
    })
  })

  it('deletes from the header', async () => {
    seedSelected()
    const { getByTestId } = await renderSelected(1)

    getByTestId('delete-message').click()

    await waitFor(() => {
      expect(useMailStore.getState().messages).toHaveLength(0)
    })
  })

  // A message the list has not loaded has no state to act on, so the header —
  // and its buttons — are not drawn at all.
  it('shows no actions when the message is not in the loaded page', async () => {
    useMailStore.setState({ messages: [] })
    const { container } = await renderSelected(42)

    expect(container.querySelector('[data-testid="toggle-read"]')).toBeNull()
  })
})

describe('move menu', () => {
  const folder = (id: number, name: string, accountId = 1) => ({
    id,
    accountId,
    name,
    path: name,
    totalCount: 0,
    unreadCount: 0,
    isInbox: name === 'INBOX',
  })

  const seedForMove = (folders: ReturnType<typeof folder>[]) =>
    useMailStore.setState({
      folders,
      messages: [
        {
          id: 1,
          folderId: 1,
          uid: 1,
          threadId: '<t1@x>',
          subject: 'Konu',
          fromName: 'Gönderen',
          fromAddr: 'g@example.com',
          snippet: 'önizleme',
          internalDateUnix: 1700000000,
          isRead: true,
          isStarred: false,
          hasAttachments: false,
          bodyFetched: false,
        },
      ],
    })

  it('offers the other folders of the same account', async () => {
    seedForMove([folder(1, 'INBOX'), folder(2, 'Arşiv'), folder(3, 'Çöp')])
    const { getByTestId, getAllByTestId } = await renderSelected(1)

    getByTestId('open-move-menu').click()

    await waitFor(() => {
      const names = getAllByTestId('move-destination').map((b) => b.textContent)
      expect(names).toEqual(['Arşiv', 'Çöp'])
    })
  })

  // A cross-account move is a copy, an upload and a delete. Offering it behind
  // the same label would be three operations hiding under one, which is how a
  // client loses mail.
  it('never offers another account as a destination', async () => {
    seedForMove([folder(1, 'INBOX'), folder(2, 'Arşiv'), folder(9, 'Other INBOX', 2)])
    const { getByTestId, getAllByTestId } = await renderSelected(1)

    getByTestId('open-move-menu').click()

    await waitFor(() => {
      const names = getAllByTestId('move-destination').map((b) => b.textContent)
      expect(names).toEqual(['Arşiv'])
    })
  })

  it('takes the message out of the list when a destination is picked', async () => {
    seedForMove([folder(1, 'INBOX'), folder(2, 'Arşiv')])
    const { getByTestId, getAllByTestId } = await renderSelected(1)

    getByTestId('open-move-menu').click()
    await waitFor(() => expect(getAllByTestId('move-destination')).toHaveLength(1))
    getAllByTestId('move-destination')[0].click()

    await waitFor(() => {
      expect(useMailStore.getState().messages).toHaveLength(0)
    })
  })

  // An account with one folder has nowhere to move to, and a button that
  // opens an empty menu is worse than no button.
  it('is not shown when there is nowhere to move to', async () => {
    seedForMove([folder(1, 'INBOX')])
    const { queryByTestId } = await renderSelected(1)

    expect(queryByTestId('open-move-menu')).toBeNull()
  })
})

describe('marking read by reading', () => {
  const unread = {
    id: 1,
    folderId: 1,
    uid: 1,
    threadId: '<t1@x>',
    subject: 'Konu',
    fromName: 'Gönderen',
    fromAddr: 'g@example.com',
    snippet: 'önizleme',
    internalDateUnix: 1700000000,
    isRead: false,
    isStarred: false,
    hasAttachments: false,
    bodyFetched: false,
  }

  it('marks the message read once its body is shown', async () => {
    useMailStore.setState({ messages: [unread] })
    await renderSelected(1)

    await waitFor(() => {
      expect(useMailStore.getState().messages[0].isRead).toBe(true)
    })
  })

  // A message that is already read must not queue a change. The reading pane
  // renders constantly, and a command per render would be thousands of
  // pointless round trips.
  it('says nothing about a message that is already read', async () => {
    useMailStore.setState({ messages: [{ ...unread, isRead: true }] })
    await renderSelected(1)

    await waitFor(() => {
      expect(useMailStore.getState().messages[0].isRead).toBe(true)
    })
  })
})

describe('source view', () => {
  const seed = () =>
    useMailStore.setState({
      messages: [
        {
          id: 1,
          folderId: 1,
          uid: 1,
          threadId: '<t1@x>',
          subject: 'Konu',
          fromName: 'Gönderen',
          fromAddr: 'g@example.com',
          snippet: 'önizleme',
          internalDateUnix: 1700000000,
          isRead: true,
          isStarred: false,
          hasAttachments: false,
          bodyFetched: false,
        },
      ],
    })

  // The source view is what a reader opens when a message rendered wrong, so
  // it has to point somewhere else entirely — not at the same sanitised body.
  it('points the frame at the source endpoint', async () => {
    seed()
    const { getByTestId, container } = await renderSelected(1)

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('toggle-source'))

    await waitFor(() => {
      const iframe = container.querySelector('iframe') as HTMLIFrameElement
      expect(iframe.getAttribute('src')).toBe('/mail-source/1')
    })
  })

  // Plain text has no remote content in it. Offering to load images over a
  // source view would be offering to do nothing.
  it('drops the remote content prompt while the source is showing', async () => {
    seed()
    const { getByTestId, queryByTestId } = await renderSelected(1)

    expect(queryByTestId('load-remote')).toBeTruthy()

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('toggle-source'))

    await waitFor(() => expect(queryByTestId('load-remote')).toBeNull())
  })

  // Somebody who opened the source to work out why one mail looked wrong does
  // not want raw headers for every message they read afterwards.
  it('goes back to the rendered body when another message is opened', async () => {
    seed()
    const { getByTestId, container } = await renderSelected(1)

    fireEvent.click(getByTestId('open-message-actions'))
    fireEvent.click(getByTestId('toggle-source'))
    await waitFor(() => {
      const iframe = container.querySelector('iframe') as HTMLIFrameElement
      expect(iframe.getAttribute('src')).toBe('/mail-source/1')
    })

    useMailStore.setState({ selectedMessageId: 2 })

    await waitFor(() => {
      const iframe = container.querySelector('iframe') as HTMLIFrameElement
      expect(iframe.getAttribute('src')).toBe('/mail-body/2')
    })
  })
})
