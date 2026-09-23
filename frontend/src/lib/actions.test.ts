import { beforeEach, describe, expect, it, vi } from 'vitest'
import { requestDelete } from './actions'
import { useMailStore } from '../store/useMailStore'
import type { Folder, Message } from './api'

const deleteMessages = vi.fn()

vi.mock('./api', () => ({
  deleteMessages: (ids: number[]) => deleteMessages(ids),
  markRead: vi.fn(),
  setStarred: vi.fn(),
  moveMessages: vi.fn(),
  undoable: vi.fn().mockResolvedValue({ kind: '', count: 0, expiresUnixMs: 0 }),
  undoLastAction: vi.fn().mockResolvedValue(false),
  redoable: vi.fn().mockResolvedValue({ kind: '', count: 0, expiresUnixMs: 0 }),
  redoLastAction: vi.fn().mockResolvedValue(false),
}))

const folder = (id: number, role: string): Folder => ({
  id,
  accountId: 1,
  name: role || 'Klasör',
  path: role || 'Klasör',
  totalCount: 0,
  unreadCount: 0,
  isInbox: role === 'inbox',
  role,
})

const message = (id: number, folderId: number): Message => ({
  id,
  folderId,
  uid: id,
  threadId: `<t${id}@x>`,
  threadCount: 1,
  subject: 'Konu',
  fromName: 'Gönderen',
  fromAddr: 'g@example.com',
  snippet: 'önizleme',
  internalDateUnix: 1700000000,
  isRead: true,
  isStarred: false,
  hasAttachments: false,
  bodyFetched: false,
})

beforeEach(() => {
  useMailStore.getState().reset()
  deleteMessages.mockReset()
  deleteMessages.mockResolvedValue(undefined)
  useMailStore.setState({
    folders: [folder(1, 'inbox'), folder(2, 'trash')],
    messages: [message(10, 1), message(20, 2)],
  })
})

describe('deciding whether to ask', () => {
  // Outside the trash the message is still somewhere the reader can find it,
  // so a question would be one they learn to dismiss without reading.
  it('deletes without asking outside the trash', async () => {
    requestDelete([10])

    expect(useMailStore.getState().pendingDelete).toBeNull()
    await vi.waitFor(() => expect(deleteMessages).toHaveBeenCalledWith([10]))
  })

  it('asks before destroying inside the trash', () => {
    requestDelete([20])

    expect(useMailStore.getState().pendingDelete).toEqual([20])
    expect(deleteMessages).not.toHaveBeenCalled()
  })

  // A selection that reaches into the trash destroys part of itself, so the
  // question is owed even though the rest is only being moved.
  it('asks when any of the selection is in the trash', () => {
    requestDelete([10, 20])

    expect(useMailStore.getState().pendingDelete).toEqual([10, 20])
    expect(deleteMessages).not.toHaveBeenCalled()
  })

  it('does nothing with an empty selection', () => {
    requestDelete([])

    expect(useMailStore.getState().pendingDelete).toBeNull()
    expect(deleteMessages).not.toHaveBeenCalled()
  })

  // A message the list no longer holds cannot be checked, and treating an
  // unknown as "in the trash" would ask about every stale selection.
  it('does not ask about a message it cannot place', async () => {
    requestDelete([999])

    expect(useMailStore.getState().pendingDelete).toBeNull()
    await vi.waitFor(() => expect(deleteMessages).toHaveBeenCalledWith([999]))
  })
})
