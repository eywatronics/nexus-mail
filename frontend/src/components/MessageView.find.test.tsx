import { act, fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { bodyURL } from '../lib/api'
import type { FindResultsPayload } from '../lib/events'
import { useMessageShortcuts } from '../lib/keyboard'
import { useMailStore } from '../store/useMailStore'
import { MessageView } from './MessageView'

// The pane learns how many matches there are from an event, because the
// counting happens while the body is served. The real runtime needs a window,
// so the subscription is stood in for and the event is delivered by hand.
const runtime = vi.hoisted(() => ({
  handlers: [] as Array<(event: { data: unknown }) => void>,
}))

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (_name: string, handler: (event: { data: unknown }) => void) => {
      runtime.handlers.push(handler)
      return () => {
        const at = runtime.handlers.indexOf(handler)
        if (at >= 0) runtime.handlers.splice(at, 1)
      }
    },
  },
}))

function announce(result: FindResultsPayload) {
  act(() => {
    runtime.handlers.forEach((handler) => handler({ data: result }))
  })
}

beforeEach(() => {
  runtime.handlers.length = 0
  useMailStore.getState().reset()
})

/** The frame's current URL, which is where the whole search lives. */
function frameURL(container: HTMLElement): string {
  return container.querySelector('iframe')?.getAttribute('src') ?? ''
}

async function openFindOn(id: number) {
  useMailStore.setState({ selectedMessageId: id, findOpen: true })
  const view = render(<MessageView />)
  const input = await waitFor(() => view.getByLabelText('Find in message'))
  return { ...view, input }
}

describe('the query in the body URL', () => {
  // The frame has no scripts, so the search cannot be run inside it. It
  // travels in the URL and comes back as a document with the matches wrapped.
  it('carries the query and the index', () => {
    const url = bodyURL(4, false, 0, 'rich', undefined, { query: 'fatura', index: 2 })

    expect(url).toContain('find=fatura')
    expect(url).toContain('findIndex=2')
  })

  // The fragment is the only way to move a document that has no scripts.
  it('points the fragment at the current match', () => {
    const url = bodyURL(4, false, 0, 'rich', undefined, { query: 'fatura', index: 0 })
    expect(url.endsWith('#nx-find-current')).toBe(true)
  })

  // Not searching has to leave the URL exactly as it was, or every message
  // anybody reads would carry the remains of a search.
  it('adds nothing when there is no query', () => {
    expect(bodyURL(4, false)).toBe('/mail-body/4')
    expect(bodyURL(4, false, 0, 'rich', undefined, { query: '  ', index: 3 })).toBe(
      '/mail-body/4',
    )
  })

  it('survives alongside consent, the view and the theme', () => {
    const url = bodyURL(9, true, 2, 'text', 'dark', { query: 'kod', index: 1 })

    for (const part of ['remote=1', 'v=2', 'view=text', 'theme=dark', 'find=kod']) {
      expect(url).toContain(part)
    }
  })
})

describe('searching a message', () => {
  it('puts the typed query in the frame URL', async () => {
    const { input, container } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })

    await waitFor(() => {
      expect(frameURL(container)).toContain('find=fatura')
    })
  })

  // The count cannot be read out of the frame, so it arrives with the render.
  it('shows the count the served document reported', async () => {
    const { input, container, getByTestId } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))

    announce({ messageId: 3, query: 'fatura', count: 4, current: 0 })

    expect(getByTestId('find-count').textContent).toBe('1/4')
  })

  // A result for another message would otherwise land while the reader had
  // already moved on, and the bar would show a count belonging to nothing on
  // screen.
  it('ignores a result for a different message', async () => {
    const { input, container, getByTestId } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))

    announce({ messageId: 99, query: 'fatura', count: 4, current: 0 })

    expect(getByTestId('find-count').textContent).not.toContain('/4')
  })

  it('ignores a result for a query already superseded', async () => {
    const { input, container, getByTestId } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))

    announce({ messageId: 3, query: 'eski', count: 9, current: 0 })

    expect(getByTestId('find-count').textContent).not.toContain('/9')
  })

  it('asks for the next match by changing the index in the URL', async () => {
    const { input, container, getByLabelText } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))
    announce({ messageId: 3, query: 'fatura', count: 3, current: 0 })

    fireEvent.click(getByLabelText('Next match'))

    await waitFor(() => expect(frameURL(container)).toContain('findIndex=1'))
  })

  // Wrapped in the window as well as the backend, so the number in the bar
  // changes with the click rather than a render later.
  it('comes round to the first match past the last', async () => {
    const { input, container, getByLabelText, getByTestId } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))
    announce({ messageId: 3, query: 'fatura', count: 2, current: 1 })

    fireEvent.click(getByLabelText('Next match'))

    expect(getByTestId('find-count').textContent).toBe('1/2')
  })

  // Closing has to take the search out of the URL, or the reader would be left
  // looking at a highlighted message with no bar explaining why.
  it('drops the search from the URL when the bar closes', async () => {
    const { input, container, getByLabelText } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))

    fireEvent.click(getByLabelText('Close find bar'))

    await waitFor(() => expect(frameURL(container)).toBe('/mail-body/3'))
  })

  // Looking for the same word through a run of messages is a real way to work,
  // so the query stays — but its count belonged to the message just left.
  it('keeps the query but not the count when another message is opened', async () => {
    const { input, container, getByTestId } = await openFindOn(3)

    fireEvent.change(input, { target: { value: 'fatura' } })
    await waitFor(() => expect(frameURL(container)).toContain('find=fatura'))
    announce({ messageId: 3, query: 'fatura', count: 4, current: 0 })

    act(() => {
      useMailStore.setState({ selectedMessageId: 8 })
    })

    await waitFor(() => expect(frameURL(container)).toContain('/mail-body/8'))
    expect((input as HTMLInputElement).value).toBe('fatura')
    expect(getByTestId('find-count').textContent).not.toContain('/4')
  })
})

/** A component that does nothing but install the global key handler. */
function Shortcuts() {
  useMessageShortcuts()
  return null
}

describe('the keyboard', () => {
  function pressCtrlF() {
    fireEvent.keyDown(window, { key: 'f', ctrlKey: true })
  }

  // The handler drops every modified keystroke, so a shortcut that wants one
  // has to sit above that guard. This is exactly how Ctrl+Z was unreachable
  // once before.
  it('opens the find bar on Ctrl+F', () => {
    useMailStore.setState({ selectedMessageId: 5 })
    render(<Shortcuts />)

    pressCtrlF()

    expect(useMailStore.getState().findOpen).toBe(true)
  })

  // Otherwise it would put a search box on screen with nothing behind it.
  it('does nothing with no message open', () => {
    render(<Shortcuts />)

    pressCtrlF()

    expect(useMailStore.getState().findOpen).toBe(false)
  })

  // Escape has to reach the bar from the message list too, not only from
  // inside the box.
  it('closes the find bar on Escape from anywhere', () => {
    useMailStore.setState({ selectedMessageId: 5, findOpen: true })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'Escape' })

    expect(useMailStore.getState().findOpen).toBe(false)
  })

  // The find bar is the nearer of the two, so Escape shuts it first and leaves
  // the search results in the list alone.
  it('leaves the search alone while the find bar is open', () => {
    useMailStore.setState({
      selectedMessageId: 5,
      findOpen: true,
      searching: true,
      searchQuery: 'fatura',
    })
    render(<Shortcuts />)

    fireEvent.keyDown(window, { key: 'Escape' })

    expect(useMailStore.getState().findOpen).toBe(false)
    expect(useMailStore.getState().searching).toBe(true)
  })
})
