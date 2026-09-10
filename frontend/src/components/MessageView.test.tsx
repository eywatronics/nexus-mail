import { render, waitFor } from '@testing-library/react'
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
