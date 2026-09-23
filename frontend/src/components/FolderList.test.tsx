import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { FolderList } from './FolderList'
import { useMailStore } from '../store/useMailStore'
import type { Folder } from '../lib/api'

const folder = (id: number, name: string, role = ''): Folder => ({
  id,
  accountId: 1,
  name,
  path: name,
  totalCount: 0,
  unreadCount: 0,
  isInbox: role === 'inbox',
  role,
})

beforeEach(() => {
  useMailStore.getState().reset()
  useMailStore.setState({
    accounts: [
      {
        id: 1,
        email: 'u@example.com',
        displayName: 'U',
        provider: 'imap',
        authKind: 'password',
      },
    ],
  })
})

describe('folder list', () => {
  // The order is decided in the backend, where the special-use attributes are.
  // Re-sorting here would be a second opinion about it, and the two would
  // eventually disagree.
  it('renders folders in the order it was given', () => {
    useMailStore.setState({
      folders: [
        folder(1, 'INBOX', 'inbox'),
        folder(2, 'Gönderilmiş Öğeler', 'sent'),
        folder(3, 'Muhasebe'),
      ],
    })

    const { getAllByTestId } = render(<FolderList />)
    const rows = getAllByTestId('folder-row')

    expect(rows.map((r) => r.textContent)).toEqual([
      'INBOX',
      'Gönderilmiş Öğeler',
      'Muhasebe',
    ])
  })

  // A Turkish sent folder is not called Sent. The icon has to come from what
  // the server said the mailbox is for, not from what it is called.
  it('gives a role its own icon whatever the folder is called', () => {
    useMailStore.setState({
      folders: [folder(1, 'Gönderilmiş Öğeler', 'sent'), folder(2, 'Muhasebe')],
    })

    const { getAllByTestId } = render(<FolderList />)
    const icons = getAllByTestId('folder-row').map((row) =>
      row.querySelector('svg')?.outerHTML,
    )

    expect(icons[0]).toBeTruthy()
    expect(icons[1]).toBeTruthy()
    expect(icons[0]).not.toBe(icons[1])
  })

  // Two folders with no role share the generic icon — that is the point of it,
  // and a test that only checked "different roles differ" would pass with
  // every folder drawn differently.
  it('draws ordinary folders with the same icon', () => {
    useMailStore.setState({
      folders: [folder(1, 'Muhasebe'), folder(2, 'Bordro')],
    })

    const { getAllByTestId } = render(<FolderList />)
    const icons = getAllByTestId('folder-row').map((row) =>
      row.querySelector('svg')?.outerHTML,
    )

    expect(icons[0]).toBe(icons[1])
  })
})
