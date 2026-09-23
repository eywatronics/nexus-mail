import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { UndoNotice } from './UndoNotice'
import { useMailStore } from '../store/useMailStore'
import type { Undoable } from '../lib/api'

const undoLastAction = vi.fn()
const undoable = vi.fn()
const redoLastAction = vi.fn()
const redoable = vi.fn()

vi.mock('../lib/api', () => ({
  deleteMessages: vi.fn(),
  markRead: vi.fn(),
  setStarred: vi.fn(),
  undoLastAction: () => undoLastAction(),
  undoable: () => undoable(),
  redoLastAction: () => redoLastAction(),
  redoable: () => redoable(),
}))

const offer = (over: Partial<Undoable> = {}): Undoable => ({
  kind: 'trash',
  count: 1,
  expiresUnixMs: Date.now() + 5000,
  ...over,
})

beforeEach(() => {
  useMailStore.getState().reset()
  undoLastAction.mockReset()
  undoLastAction.mockResolvedValue(true)
  undoable.mockReset()
  undoable.mockResolvedValue({ kind: '', count: 0, expiresUnixMs: 0 })
  redoLastAction.mockReset()
  redoLastAction.mockResolvedValue(true)
  redoable.mockReset()
  redoable.mockResolvedValue({ kind: '', count: 0, expiresUnixMs: 0 })
})

describe('the undo offer', () => {
  it('shows nothing when there is nothing to take back', () => {
    const { container } = render(<UndoNotice />)
    expect(container.firstChild).toBeNull()
  })

  // "Undo" on its own asks the reader to remember what they just did, and the
  // case this exists for is precisely the one where they did not mean to.
  it('says what happened, not only that something can be undone', () => {
    useMailStore.setState({ undoOffer: offer() })
    const { getByTestId } = render(<UndoNotice />)

    expect(getByTestId('undo-notice').textContent).toContain('Moved to trash')
  })

  it('counts the messages when there is more than one', () => {
    useMailStore.setState({ undoOffer: offer({ count: 4 }) })
    const { getByTestId } = render(<UndoNotice />)

    expect(getByTestId('undo-notice').textContent).toContain('4 messages moved to trash')
  })

  it('describes a move differently from a delete', () => {
    useMailStore.setState({ undoOffer: offer({ kind: 'move' }) })
    const { getByTestId, rerender } = render(<UndoNotice />)
    expect(getByTestId('undo-notice').textContent).toContain('Message moved')

    useMailStore.setState({ undoOffer: offer({ kind: 'delete' }) })
    rerender(<UndoNotice />)
    expect(getByTestId('undo-notice').textContent).toContain('Message deleted')
  })

  it('takes the action back when pressed', async () => {
    useMailStore.setState({ undoOffer: offer() })
    const { getByTestId } = render(<UndoNotice />)

    fireEvent.click(getByTestId('undo-action'))

    await waitFor(() => expect(undoLastAction).toHaveBeenCalled())
  })

  // A button that sometimes silently does nothing is worse than no button, so
  // the offer goes away the moment it is taken.
  it('disappears once it has been used', async () => {
    useMailStore.setState({ undoOffer: offer() })
    const { getByTestId, queryByTestId } = render(<UndoNotice />)

    fireEvent.click(getByTestId('undo-action'))

    await waitFor(() => expect(queryByTestId('undo-notice')).toBeNull())
  })

  // An unrecognised kind is a backend this window does not understand. Drawing
  // an empty strip would be worse than drawing nothing.
  it('draws nothing for a kind it does not know', () => {
    useMailStore.setState({
      undoOffer: { kind: 'something-else', count: 1, expiresUnixMs: Date.now() + 5000 } as unknown as Undoable,
    })
    const { container } = render(<UndoNotice />)

    expect(container.firstChild).toBeNull()
  })
})

describe('the way back from an undo', () => {
  // "Undone" on its own is no better than a bare "Undo": it says something
  // changed without saying what, in the moment the reader is least sure.
  it('says what the undo did', () => {
    useMailStore.setState({ redoOffer: offer() })
    const { getByTestId } = render(<UndoNotice />)

    expect(getByTestId('undo-notice').textContent).toContain('Taken back out of the trash')
    expect(getByTestId('redo-action').textContent).toContain('Redo')
  })

  it('describes an undone move differently from an undone delete', () => {
    useMailStore.setState({ redoOffer: offer({ kind: 'move' }) })
    const { getByTestId, rerender } = render(<UndoNotice />)
    expect(getByTestId('undo-notice').textContent).toContain('Move undone')

    useMailStore.setState({ redoOffer: offer({ kind: 'delete' }) })
    rerender(<UndoNotice />)
    expect(getByTestId('undo-notice').textContent).toContain('Deletion undone')
  })

  it('does the action again when pressed', async () => {
    useMailStore.setState({ redoOffer: offer() })
    const { getByTestId } = render(<UndoNotice />)

    fireEvent.click(getByTestId('redo-action'))

    await waitFor(() => expect(redoLastAction).toHaveBeenCalled())
  })

  // Taking the undo is what creates the redo, so the two are never live
  // together — but if a race made them overlap, the more recent action is the
  // one the reader is thinking about.
  it('shows the undo rather than the redo if both are somehow live', () => {
    useMailStore.setState({ undoOffer: offer(), redoOffer: offer({ kind: 'move' }) })
    const { getByTestId, queryByTestId } = render(<UndoNotice />)

    expect(getByTestId('undo-notice').textContent).toContain('Moved to trash')
    expect(queryByTestId('redo-action')).toBeNull()
  })

  // Pressing undo has to leave the way back on screen rather than an empty
  // corner: the reader who undid by reflex and thought better of it is the
  // same reader this exists for, one step further along.
  it('appears after an undo is taken', async () => {
    redoable.mockResolvedValue(offer({ kind: 'move' }))
    useMailStore.setState({ undoOffer: offer() })
    const { getByTestId } = render(<UndoNotice />)

    fireEvent.click(getByTestId('undo-action'))

    await waitFor(() => expect(getByTestId('redo-action')).toBeTruthy())
  })

  // An undo the backend refused took nothing back, so there is nothing to put
  // back either. Offering a redo would be offering to repeat an action that
  // was never reversed.
  it('does not appear when the undo came back empty-handed', async () => {
    undoLastAction.mockResolvedValue(false)
    redoable.mockResolvedValue(offer({ kind: 'move' }))
    useMailStore.setState({ undoOffer: offer() })
    const { getByTestId, queryByTestId } = render(<UndoNotice />)

    fireEvent.click(getByTestId('undo-action'))

    await waitFor(() => expect(undoLastAction).toHaveBeenCalled())
    expect(queryByTestId('redo-action')).toBeNull()
  })
})
