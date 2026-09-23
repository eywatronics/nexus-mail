import { render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AttachmentList } from './AttachmentList'

const listAttachments = vi.fn()
const revealAttachment = vi.fn()

vi.mock('../lib/api', () => ({
  listAttachments: (id: number) => listAttachments(id),
  revealAttachment: (id: number) => revealAttachment(id),
}))

const file = (id: number, filename: string, size = 2048) => ({
  id,
  filename,
  mimeType: 'application/pdf',
  size,
  downloaded: false,
})

beforeEach(() => {
  listAttachments.mockReset()
  revealAttachment.mockReset()
  revealAttachment.mockResolvedValue(undefined)
})

describe('attachment list', () => {
  it('shows a button per file with its name and size', async () => {
    listAttachments.mockResolvedValue([file(1, 'rapor.pdf', 2048), file(2, 'tablo.xlsx', 512)])

    const { findAllByTestId } = render(<AttachmentList messageId={7} />)
    const buttons = await findAllByTestId('attachment')

    expect(buttons).toHaveLength(2)
    expect(buttons[0].textContent).toContain('rapor.pdf')
    expect(buttons[0].textContent).toContain('2 KB')
  })

  // Nothing to show means nothing drawn: an empty bar with a lone paperclip
  // would be a row of chrome saying "there are no files here".
  it('draws nothing when the message carries no files', async () => {
    listAttachments.mockResolvedValue([])

    const { queryByTestId } = render(<AttachmentList messageId={7} />)
    await waitFor(() => expect(listAttachments).toHaveBeenCalled())

    expect(queryByTestId('attachment-list')).toBeNull()
  })

  it('asks the backend to fetch and reveal the file when clicked', async () => {
    listAttachments.mockResolvedValue([file(1, 'rapor.pdf')])

    const { findAllByTestId } = render(<AttachmentList messageId={7} />)
    const [button] = await findAllByTestId('attachment')
    button.click()

    await waitFor(() => expect(revealAttachment).toHaveBeenCalledWith(1))
  })

  // The reader can move on before the list arrives. Writing the previous
  // message's files into the pane would be worse than showing none.
  it('does not show a previous message list after the message changed', async () => {
    let resolveFirst: (v: unknown) => void = () => {}
    listAttachments.mockImplementationOnce(
      () => new Promise((resolve) => { resolveFirst = resolve }),
    )
    listAttachments.mockResolvedValueOnce([file(9, 'ikinci.pdf')])

    const { rerender, findAllByTestId } = render(<AttachmentList messageId={1} />)
    rerender(<AttachmentList messageId={2} />)
    resolveFirst([file(1, 'birinci.pdf')])

    const buttons = await findAllByTestId('attachment')
    expect(buttons).toHaveLength(1)
    expect(buttons[0].textContent).toContain('ikinci.pdf')
  })
})
