import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ConfirmDeleteDialog } from './ConfirmDeleteDialog'
import { useMailStore } from '../store/useMailStore'

const deleteMessages = vi.fn()

vi.mock('../lib/api', () => ({
  deleteMessages: (ids: number[]) => deleteMessages(ids),
  markRead: vi.fn(),
  setStarred: vi.fn(),
  moveMessages: vi.fn(),
}))

beforeEach(() => {
  useMailStore.getState().reset()
  deleteMessages.mockReset()
  deleteMessages.mockResolvedValue(undefined)
})

describe('confirming a permanent delete', () => {
  // Everywhere but the trash, deleting moves the message there and needs no
  // question — the message is still somewhere the reader can find it.
  it('shows nothing when nothing is waiting on an answer', () => {
    const { container } = render(<ConfirmDeleteDialog />)
    expect(container.firstChild).toBeNull()
  })

  it('names the server, because that is what makes this press different', () => {
    useMailStore.setState({ pendingDelete: [1] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    expect(getByTestId('confirm-dialog').textContent).toContain('server')
    expect(getByTestId('confirm-dialog').textContent).toContain('cannot be undone')
  })

  it('counts the messages when there is more than one', () => {
    useMailStore.setState({ pendingDelete: [1, 2, 3] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    expect(getByTestId('confirm-dialog').textContent).toContain('3 messages')
  })

  it('destroys only after the answer', async () => {
    useMailStore.setState({ pendingDelete: [7] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    expect(deleteMessages).not.toHaveBeenCalled()
    fireEvent.click(getByTestId('confirm-accept'))

    await waitFor(() => expect(deleteMessages).toHaveBeenCalledWith([7]))
    expect(useMailStore.getState().pendingDelete).toBeNull()
  })

  it('destroys nothing when the answer is no', () => {
    useMailStore.setState({ pendingDelete: [7] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    fireEvent.click(getByTestId('confirm-cancel'))

    expect(deleteMessages).not.toHaveBeenCalled()
    expect(useMailStore.getState().pendingDelete).toBeNull()
  })

  // Somebody who arrived here by pressing a key twice gets the harmless
  // outcome from the next reflex.
  it('backs out on Escape', async () => {
    useMailStore.setState({ pendingDelete: [7] })
    render(<ConfirmDeleteDialog />)

    fireEvent.keyDown(window, { key: 'Escape' })

    await waitFor(() => expect(useMailStore.getState().pendingDelete).toBeNull())
    expect(deleteMessages).not.toHaveBeenCalled()
  })

  // Clicking outside a dialog to dismiss it is what everyone tries first.
  it('backs out when the backdrop is clicked', () => {
    useMailStore.setState({ pendingDelete: [7] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    fireEvent.click(getByTestId('confirm-backdrop'))

    expect(useMailStore.getState().pendingDelete).toBeNull()
    expect(deleteMessages).not.toHaveBeenCalled()
  })

  // A click inside must not travel to the backdrop and dismiss the thing the
  // reader is reading.
  it('stays open when the dialog itself is clicked', () => {
    useMailStore.setState({ pendingDelete: [7] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    fireEvent.click(getByTestId('confirm-dialog'))

    expect(useMailStore.getState().pendingDelete).toEqual([7])
  })

  // Cancel takes the focus, not confirm: the next reflex has to be the
  // harmless one.
  it('puts the focus on the harmless button', () => {
    useMailStore.setState({ pendingDelete: [7] })
    const { getByTestId } = render(<ConfirmDeleteDialog />)

    expect(document.activeElement).toBe(getByTestId('confirm-cancel'))
  })
})
