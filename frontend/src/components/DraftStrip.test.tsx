import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DraftStrip } from './DraftStrip'
import { useMailStore } from '../store/useMailStore'
import type { DraftRecord, Folder } from '../lib/api'

const drafts = vi.fn()
const discardDraft = vi.fn()

vi.mock('../lib/api', () => ({
  drafts: (accountId: number) => drafts(accountId),
  discardDraft: (id: number) => discardDraft(id),
}))

const folder = (over: Partial<Folder> = {}): Folder => ({
  id: 1,
  accountId: 9,
  name: 'Drafts',
  path: 'Drafts',
  totalCount: 0,
  unreadCount: 0,
  isInbox: false,
  role: 'drafts',
  ...over,
})

const record = (over: Partial<DraftRecord> = {}): DraftRecord => ({
  id: 4,
  accountId: 9,
  identityId: 0,
  to: '',
  cc: '',
  bcc: '',
  subject: 'Yarim konu',
  body: 'metin',
  inReplyTo: '',
  references: [],
  attachmentPaths: [],
  updatedAtUnix: 1_700_000_000,
  ...over,
})

function show(folders: Folder[], selected: number | null) {
  useMailStore.setState({ folders, selectedFolderId: selected, composing: null })
  return render(<DraftStrip />)
}

beforeEach(() => {
  useMailStore.getState().reset()
  drafts.mockReset()
  drafts.mockResolvedValue([record()])
  discardDraft.mockReset()
  discardDraft.mockResolvedValue(undefined)
})

describe('unsent drafts', () => {
  it('shows them above the Drafts folder', async () => {
    const { findByTestId } = show([folder()], 1)

    expect(await findByTestId('draft-strip')).toBeTruthy()
    await waitFor(() => expect(drafts).toHaveBeenCalledWith(9))
  })

  // A draft belongs in Drafts. Showing the strip anywhere else would be a
  // second place for the same thing.
  it('shows nothing over any other folder', async () => {
    const { queryByTestId } = show([folder({ id: 2, role: 'inbox', name: 'Inbox' })], 2)

    await waitFor(() => expect(queryByTestId('draft-strip')).toBeNull())
    expect(drafts).not.toHaveBeenCalled()
  })

  it('shows nothing when there is nothing unsent', async () => {
    drafts.mockResolvedValue([])
    const { queryByTestId } = show([folder()], 1)

    await waitFor(() => expect(drafts).toHaveBeenCalled())
    expect(queryByTestId('draft-strip')).toBeNull()
  })

  it('opens one in the composer', async () => {
    const { findByTestId } = show([folder()], 1)

    fireEvent.click(await findByTestId('draft-open'))

    const composing = useMailStore.getState().composing
    expect(composing?.accountId).toBe(9)
    expect(composing?.draft?.id).toBe(4)
  })

  // An untitled draft still has to be clickable.
  it('gives an untitled draft something to click', async () => {
    drafts.mockResolvedValue([record({ subject: '   ' })])
    const { findByText } = show([folder()], 1)

    expect(await findByText('No subject')).toBeTruthy()
  })

  it('throws one away and stops showing it', async () => {
    const { findByLabelText, queryByTestId } = show([folder()], 1)

    drafts.mockResolvedValue([])
    fireEvent.click(await findByLabelText('Discard Yarim konu'))

    await waitFor(() => expect(discardDraft).toHaveBeenCalledWith(4))
    await waitFor(() => expect(queryByTestId('draft-strip')).toBeNull())
  })

  // This is a convenience above a folder, not the folder itself. An error
  // banner here would sit over mail the person can still read perfectly well.
  it('stays quiet when it cannot be filled', async () => {
    drafts.mockRejectedValue(new Error('nope'))
    const { queryByTestId } = show([folder()], 1)

    await waitFor(() => expect(drafts).toHaveBeenCalled())
    expect(queryByTestId('draft-strip')).toBeNull()
  })
})
