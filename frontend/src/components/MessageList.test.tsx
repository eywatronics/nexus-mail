import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMailStore } from '../store/useMailStore'
import { MessageList } from './MessageList'
import type { Message } from '../lib/api'

function makeMessages(count: number): Message[] {
  return Array.from({ length: count }, (_, i) => ({
    id: i + 1,
    folderId: 1,
    uid: i + 1,
    threadId: `<t${i}@x>`,
    subject: `Subject ${i + 1}`,
    fromName: `Sender ${i + 1}`,
    fromAddr: `s${i + 1}@example.com`,
    snippet: 'preview text',
    internalDateUnix: 1700000000 + i,
    isRead: i % 2 === 0,
    isStarred: false,
    hasAttachments: false,
    bodyFetched: false,
  }))
}

beforeEach(() => {
  useMailStore.getState().reset()
})

describe('virtualisation', () => {
  // The project's headline claim is that a 50,000-message mailbox scrolls
  // smoothly. That only holds if the DOM stays small, so assert the DOM size
  // rather than trusting the library to behave.
  it('renders a bounded number of rows for 50,000 messages', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(50_000) })

    const { container } = render(<MessageList onLoadMore={() => {}} />)

    const rows = container.querySelectorAll('[data-testid="message-row"]')
    expect(rows.length).toBeGreaterThan(0)
    expect(rows.length).toBeLessThan(100)

    // A bounded row count inside a huge wrapper tree would still be slow.
    expect(container.querySelectorAll('*').length).toBeLessThan(1000)
  })
})

describe('empty states', () => {
  it('prompts for a folder when none is selected', () => {
    useMailStore.setState({ selectedFolderId: null, messages: [] })
    render(<MessageList onLoadMore={() => {}} />)
    expect(screen.getByText(/select a folder/i)).toBeTruthy()
  })

  it('shows an empty state rather than a blank pane', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: [] })
    render(<MessageList onLoadMore={() => {}} />)
    expect(screen.getByText(/no messages/i)).toBeTruthy()
  })

  it('says it is loading rather than claiming the folder is empty', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: [], loadingMessages: true })
    render(<MessageList onLoadMore={() => {}} />)
    expect(screen.getByText(/loading/i)).toBeTruthy()
  })
})

describe('rows', () => {
  it('marks unread rows so they are visually distinct', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(4) })
    const { container } = render(<MessageList onLoadMore={() => {}} />)
    expect(container.querySelectorAll('[data-unread="true"]').length).toBeGreaterThan(0)
  })

  it('selects a message when its row is clicked', () => {
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(5) })
    const { container } = render(<MessageList onLoadMore={() => {}} />)

    const firstRow = container.querySelector('[data-testid="message-row"]') as HTMLElement
    firstRow.click()

    expect(useMailStore.getState().selectedMessageId).not.toBeNull()
  })
})

describe('pagination', () => {
  it('asks for more when the end of the list is reached', () => {
    const onLoadMore = vi.fn()
    // A short list means the last row renders immediately, which is what
    // triggers the request.
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(3), hasMore: true })
    render(<MessageList onLoadMore={onLoadMore} />)
    expect(onLoadMore).toHaveBeenCalled()
  })

  it('does not ask when there is nothing left', () => {
    const onLoadMore = vi.fn()
    useMailStore.setState({ selectedFolderId: 1, messages: makeMessages(3), hasMore: false })
    render(<MessageList onLoadMore={onLoadMore} />)
    expect(onLoadMore).not.toHaveBeenCalled()
  })

  it('does not ask again while a page is already in flight', () => {
    const onLoadMore = vi.fn()
    useMailStore.setState({
      selectedFolderId: 1,
      messages: makeMessages(3),
      hasMore: true,
      loadingMessages: true,
    })
    render(<MessageList onLoadMore={onLoadMore} />)
    expect(onLoadMore).not.toHaveBeenCalled()
  })
})

describe('selection is visible', () => {
  // The accent bar was drawn in the divider's grey and nobody could see which
  // row was selected. `border-neutral-200` sets the colour of all four sides
  // and `border-l-2` only sets a width, so two rules of equal specificity were
  // both writing the left border's colour and Tailwind's emission order
  // decided the winner. jsdom does not resolve the stylesheet, so this asserts
  // the shape of the classes instead: a row must colour its bottom edge
  // specifically, never all four.
  it('colours the left bar with the accent and the divider only on the bottom', () => {
    useMailStore.setState({
      selectedFolderId: 1,
      messages: makeMessages(3),
      selectedMessageId: 1,
    })

    const { container } = render(<MessageList onLoadMore={() => {}} />)
    const rows = container.querySelectorAll('[data-testid="message-row"]')
    const selected = container.querySelector('[aria-current="true"]') as HTMLElement

    expect(selected).toBeTruthy()
    expect(selected.className).toContain('border-l-[var(--color-accent)]')

    for (const row of rows) {
      // border-neutral-* would colour every side, including the one the accent
      // bar needs. border-b-neutral-* is the correct, single-sided form.
      expect(row.className).not.toMatch(/(?<!-[a-z])border-neutral-/)
      expect(row.className).toContain('border-b-neutral-')
    }
  })

  // Unselected rows reserve the same 2px, otherwise selecting a row shifts its
  // text sideways.
  it('reserves the bar width on unselected rows', () => {
    useMailStore.setState({
      selectedFolderId: 1,
      messages: makeMessages(3),
      selectedMessageId: 1,
    })

    const { container } = render(<MessageList onLoadMore={() => {}} />)
    const unselected = container.querySelector('[aria-current="false"]') as HTMLElement

    expect(unselected.className).toContain('border-l-2')
    expect(unselected.className).toContain('border-l-transparent')
  })
})
