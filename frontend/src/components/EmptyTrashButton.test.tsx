import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { EmptyTrashButton } from './EmptyTrashButton'
import { useMailStore } from '../store/useMailStore'
import type { Folder } from '../lib/api'

const emptyTrash = vi.fn()

vi.mock('../lib/api', () => ({
  emptyTrash: (accountId: number) => emptyTrash(accountId),
}))

const folder = (id: number, role: string, totalCount = 0): Folder => ({
  id,
  accountId: 7,
  name: role === 'trash' ? 'Çöp Kutusu' : 'INBOX',
  path: role === 'trash' ? 'Çöp Kutusu' : 'INBOX',
  totalCount,
  unreadCount: 0,
  isInbox: role === 'inbox',
  role,
})

const showing = (f: Folder) =>
  useMailStore.setState({ folders: [f], selectedFolderId: f.id })

beforeEach(() => {
  useMailStore.getState().reset()
  emptyTrash.mockReset()
  emptyTrash.mockResolvedValue(undefined)
})

describe('emptying the trash', () => {
  // A control that destroyed a folder's contents from anywhere in the window
  // would be one click away from a reader who is not looking at what it would
  // destroy.
  it('is not offered outside the trash', () => {
    showing(folder(1, 'inbox', 20))
    const { container } = render(<EmptyTrashButton />)

    expect(container.firstChild).toBeNull()
  })

  it('is offered inside the trash', () => {
    showing(folder(2, 'trash', 3))
    const { getByTestId } = render(<EmptyTrashButton />)

    expect(getByTestId('empty-trash')).toBeTruthy()
  })

  it('is disabled when there is nothing to empty', () => {
    showing(folder(2, 'trash', 0))
    const { getByTestId } = render(<EmptyTrashButton />)

    expect((getByTestId('empty-trash') as HTMLButtonElement).disabled).toBe(true)
  })

  it('asks before destroying anything', () => {
    showing(folder(2, 'trash', 3))
    const { getByTestId } = render(<EmptyTrashButton />)

    fireEvent.click(getByTestId('empty-trash'))

    expect(getByTestId('confirm-dialog')).toBeTruthy()
    expect(emptyTrash).not.toHaveBeenCalled()
  })

  // The count comes from the folder row, which is the server's, not from the
  // rows this window happens to have loaded.
  it('names the server count, not what is on screen', () => {
    showing(folder(2, 'trash', 8000))
    const { getByTestId } = render(<EmptyTrashButton />)

    fireEvent.click(getByTestId('empty-trash'))

    const dialog = getByTestId('confirm-dialog')
    expect(dialog.textContent).toContain('8000 messages')
    expect(dialog.textContent).toContain('has not downloaded')
  })

  it('empties the account the folder belongs to', async () => {
    showing(folder(2, 'trash', 3))
    const { getByTestId } = render(<EmptyTrashButton />)

    fireEvent.click(getByTestId('empty-trash'))
    fireEvent.click(getByTestId('confirm-accept'))

    await waitFor(() => expect(emptyTrash).toHaveBeenCalledWith(7))
  })

  it('destroys nothing when the answer is no', () => {
    showing(folder(2, 'trash', 3))
    const { getByTestId, queryByTestId } = render(<EmptyTrashButton />)

    fireEvent.click(getByTestId('empty-trash'))
    fireEvent.click(getByTestId('confirm-cancel'))

    expect(queryByTestId('confirm-dialog')).toBeNull()
    expect(emptyTrash).not.toHaveBeenCalled()
  })
})
