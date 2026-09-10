import { fireEvent, render, screen } from '@testing-library/react'
import { renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { useMailStore } from '../store/useMailStore'
import { useMessageShortcuts } from '../lib/keyboard'
import { MessageList } from './MessageList'
import { SearchBox } from './SearchBox'
import type { Folder, Message } from '../lib/api'

const message = (id: number, folderId = 1): Message => ({
  id,
  folderId,
  uid: id,
  threadId: `<t${id}@x>`,
  subject: `Konu ${id}`,
  fromName: 'Gönderen',
  fromAddr: 'g@example.com',
  snippet: 'önizleme',
  internalDateUnix: 1700000000 + id,
  isRead: true,
  isStarred: false,
  hasAttachments: false,
  bodyFetched: false,
})

const folder = (id: number, name: string): Folder => ({
  id,
  accountId: 1,
  name,
  path: name,
  totalCount: 0,
  unreadCount: 0,
  isInbox: name === 'INBOX',
})

beforeEach(() => {
  useMailStore.getState().reset()
})

describe('search box', () => {
  it('puts what is typed into the store', () => {
    render(<SearchBox />)

    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'mutabakat' } })

    expect(useMailStore.getState().searchQuery).toBe('mutabakat')
    expect(useMailStore.getState().searching).toBe(true)
  })

  // "/" is the shortcut every reader-shaped application uses. Someone reaching
  // for it from the message list should land in the box.
  it('takes focus when slash is pressed anywhere in the window', () => {
    render(<SearchBox />)
    const input = screen.getByRole('searchbox')

    expect(document.activeElement).not.toBe(input)
    fireEvent.keyDown(window, { key: '/' })

    expect(document.activeElement).toBe(input)
  })

  // Otherwise the shortcut makes the box impossible to type a slash into,
  // which matters the moment someone searches for a path or a URL.
  it('lets a slash be typed into the box itself', () => {
    render(<SearchBox />)
    const input = screen.getByRole('searchbox') as HTMLInputElement
    input.focus()

    fireEvent.keyDown(input, { key: '/' })
    fireEvent.change(input, { target: { value: 'a/b' } })

    expect(useMailStore.getState().searchQuery).toBe('a/b')
  })

  it('clears on Escape', () => {
    render(<SearchBox />)
    const input = screen.getByRole('searchbox')

    fireEvent.change(input, { target: { value: 'mutabakat' } })
    fireEvent.keyDown(input, { key: 'Escape' })

    expect(useMailStore.getState().searchQuery).toBe('')
    expect(useMailStore.getState().searching).toBe(false)
  })

  it('offers a clear button only once there is something to clear', () => {
    render(<SearchBox />)
    expect(screen.queryByTestId('clear-search')).toBeNull()

    fireEvent.change(screen.getByRole('searchbox'), { target: { value: 'x' } })
    screen.getByTestId('clear-search').click()

    expect(useMailStore.getState().searchQuery).toBe('')
  })

  it('shows progress while a query is in flight', () => {
    useMailStore.setState({ searchQuery: 'x', searching: true, searchPending: true })
    render(<SearchBox />)

    expect(screen.getByTestId('search-pending')).toBeTruthy()
  })
})

describe('search results in the list', () => {
  // Results cross folders, so a row that does not name its folder leaves the
  // reader unable to tell a draft from the copy that was actually sent.
  it('names the folder each result came from', () => {
    useMailStore.setState({
      selectedFolderId: 1,
      folders: [folder(1, 'INBOX'), folder(2, 'Arşiv')],
      messages: [message(1, 1)],
      searchQuery: 'konu',
      searching: true,
      searchResults: [message(5, 2)],
    })

    render(<MessageList onLoadMore={() => {}} />)

    expect(screen.getByTestId('result-folder').textContent).toBe('Arşiv')
  })

  it('does not label rows when no search is running', () => {
    useMailStore.setState({
      selectedFolderId: 1,
      folders: [folder(1, 'INBOX')],
      messages: [message(1, 1)],
    })

    render(<MessageList onLoadMore={() => {}} />)

    expect(screen.queryByTestId('result-folder')).toBeNull()
  })

  // "No messages in this folder" is a lie when the folder has plenty and the
  // search is what came up empty.
  it('distinguishes an empty result from an empty folder', () => {
    useMailStore.setState({
      selectedFolderId: 1,
      messages: [message(1)],
      searchQuery: 'yok',
      searching: true,
      searchResults: [],
    })

    render(<MessageList onLoadMore={() => {}} />)

    expect(screen.getByText(/nothing matches/i)).toBeTruthy()
  })

  // Results are one capped page. Asking for more would page through the
  // folder's messages while showing search results.
  it('never asks for another page of results', () => {
    let asked = false
    useMailStore.setState({
      selectedFolderId: 1,
      messages: [],
      hasMore: true,
      searchQuery: 'konu',
      searching: true,
      searchResults: [message(1), message(2)],
    })

    render(<MessageList onLoadMore={() => { asked = true }} />)

    expect(asked).toBe(false)
  })
})

describe('keyboard navigation', () => {
  const seed = (count: number) =>
    useMailStore.setState({
      selectedFolderId: 1,
      messages: Array.from({ length: count }, (_, i) => message(i + 1)),
    })

  it('moves down with j and up with k', () => {
    renderHook(() => useMessageShortcuts())
    seed(3)

    fireEvent.keyDown(window, { key: 'j' })
    expect(useMailStore.getState().selectedMessageId).toBe(1)

    fireEvent.keyDown(window, { key: 'j' })
    expect(useMailStore.getState().selectedMessageId).toBe(2)

    fireEvent.keyDown(window, { key: 'k' })
    expect(useMailStore.getState().selectedMessageId).toBe(1)
  })

  it('moves with the arrow keys too', () => {
    renderHook(() => useMessageShortcuts())
    seed(3)

    fireEvent.keyDown(window, { key: 'ArrowDown' })
    fireEvent.keyDown(window, { key: 'ArrowDown' })
    expect(useMailStore.getState().selectedMessageId).toBe(2)

    fireEvent.keyDown(window, { key: 'ArrowUp' })
    expect(useMailStore.getState().selectedMessageId).toBe(1)
  })

  // The single most likely way to ship this broken: typing "j" in the search
  // box also jumps the list.
  it('ignores keystrokes aimed at a text field', () => {
    renderHook(() => useMessageShortcuts())
    render(<SearchBox />)
    seed(3)

    const input = screen.getByRole('searchbox')
    input.focus()
    fireEvent.keyDown(input, { key: 'j' })

    expect(useMailStore.getState().selectedMessageId).toBeNull()
  })

  // Ctrl+ArrowDown belongs to the window or the platform, not to the list.
  it('leaves modified keystrokes alone', () => {
    renderHook(() => useMessageShortcuts())
    seed(3)

    fireEvent.keyDown(window, { key: 'ArrowDown', ctrlKey: true })
    fireEvent.keyDown(window, { key: 'j', metaKey: true })

    expect(useMailStore.getState().selectedMessageId).toBeNull()
  })

  it('leaves the search from the list, not just the box', () => {
    renderHook(() => useMessageShortcuts())
    useMailStore.setState({ searchQuery: 'konu', searching: true })

    fireEvent.keyDown(window, { key: 'Escape' })

    expect(useMailStore.getState().searching).toBe(false)
  })
})
