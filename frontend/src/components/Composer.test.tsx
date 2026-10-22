import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Composer } from './Composer'
import type { DraftRecord, Identity, OutgoingAttachment } from '../lib/api'

const identities = vi.fn()
const sendMessage = vi.fn()
const pickAttachments = vi.fn()
const pickInlineImage = vi.fn()
const saveDraft = vi.fn()
const suggestRecipients = vi.fn()

vi.mock('../lib/api', () => ({
  identities: (accountId: number) => identities(accountId),
  sendMessage: (draft: unknown) => sendMessage(draft),
  pickAttachments: () => pickAttachments(),
  pickInlineImage: () => pickInlineImage(),
  saveDraft: (draft: unknown) => saveDraft(draft),
  suggestRecipients: (...args: unknown[]) => suggestRecipients(...args),
}))

const identity = (over: Partial<Identity> = {}): Identity => ({
  id: 1,
  accountId: 1,
  email: 'u@example.com',
  displayName: 'Yazan',
  from: 'Yazan <u@example.com>',
  isDefault: true,
  ...over,
})

beforeEach(() => {
  identities.mockReset()
  identities.mockResolvedValue([identity()])
  sendMessage.mockReset()
  sendMessage.mockResolvedValue({ operationId: 1, recipients: 1 })
  pickAttachments.mockReset()
  pickAttachments.mockResolvedValue([])
  pickInlineImage.mockReset()
  pickInlineImage.mockResolvedValue({
    path: '', cid: '', name: '', mimeType: '', url: '',
  })
  saveDraft.mockReset()
  saveDraft.mockResolvedValue(7)
  suggestRecipients.mockReset()
  suggestRecipients.mockResolvedValue([])
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: vi.fn(),
  })
  vi.useRealTimers()
})

const file = (over: Partial<OutgoingAttachment> = {}): OutgoingAttachment => ({
  path: '/tmp/rapor.pdf',
  name: 'rapor.pdf',
  size: 2048,
  mimeType: 'application/pdf',
  ...over,
})

const record = (over: Partial<DraftRecord> = {}): DraftRecord => ({
  id: 7,
  accountId: 1,
  identityId: 0,
  to: 'ali@example.com, bir yar',
  cc: '',
  bcc: '',
  subject: 'Yarim konu',
  body: 'burada kalmis',
  html: '',
  inReplyTo: '',
  references: [],
  attachmentPaths: [],
  inlineImages: [],
  updatedAtUnix: 1_700_000_000,
  ...over,
})

function open(props: Partial<Parameters<typeof Composer>[0]> = {}) {
  const onClose = vi.fn()
  return {
    onClose,
    ...render(<Composer accountId={1} onClose={onClose} {...props} />),
  }
}

describe('writing a message', () => {
  it('hands the fields to the backend as typed', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalledWith(1))

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.change(getByTestId('composer-subject'), { target: { value: 'Konu' } })
    fireEvent.change(getByTestId('composer-body'), { target: { value: 'gövde' } })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          accountId: 1,
          to: 'r@example.com',
          subject: 'Konu',
          text: 'gövde',
        }),
      ),
    )
  })

  // Addresses are parsed in one place, in the backend. The composer sending
  // them as typed is what makes that true.
  it('does not try to parse the address line itself', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), {
      target: { value: 'Bir Kişi <a@example.com>, b@example.com' },
    })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({ to: 'Bir Kişi <a@example.com>, b@example.com' }),
      ),
    )
  })

  it('closes once the message is queued', async () => {
    const { getByTestId, onClose } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() => expect(onClose).toHaveBeenCalled())
  })
})

describe('formatted text', () => {
  it('switches to the rich editor and back', async () => {
    const { getByTestId, queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-body'), {
      target: { value: 'bir\n\niki' },
    })
    fireEvent.click(getByTestId('composer-format'))

    expect(getByTestId('rich-editor').innerHTML).toBe('<p>bir</p><p>iki</p>')
    expect(queryByTestId('composer-body')).toBeNull()

    fireEvent.click(getByTestId('composer-format'))

    expect((getByTestId('composer-body') as HTMLTextAreaElement).value).toBe(
      'bir\n\niki',
    )
  })

  it('sends HTML and its plain text alternative', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.click(getByTestId('composer-format'))

    const editor = getByTestId('rich-editor')
    editor.innerHTML = '<p>kalın <b>metin</b></p>'
    fireEvent.input(editor)
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          html: '<p>kalın <b>metin</b></p>',
          text: 'kalın metin',
        }),
      ),
    )
  })

  it('saves the formatting with the draft', async () => {
    vi.useFakeTimers()
    const { getByTestId } = open()
    await vi.waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-format'))
    const editor = getByTestId('rich-editor')
    editor.innerHTML = '<p><b>kalın</b></p>'
    fireEvent.input(editor)

    await vi.advanceTimersByTimeAsync(2100)

    expect(saveDraft).toHaveBeenLastCalledWith(
      expect.objectContaining({ body: 'kalın', html: '<p><b>kalın</b></p>' }),
    )
  })

  it('reopens a formatted draft in the editor', async () => {
    const { getByTestId, queryByTestId } = open({
      draft: record({ body: 'kalın', html: '<p><b>kalın</b></p>' }),
    })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect(getByTestId('rich-editor').innerHTML).toBe('<p><b>kalın</b></p>')
    expect(queryByTestId('composer-body')).toBeNull()
  })

  it('reopens an embedded image as a preview', async () => {
    const image = {
      path: '/tmp/grafik.png',
      cid: 'nx-1@nexus-mail',
      name: 'grafik.png',
      mimeType: 'image/png',
      url: '/compose-image/nx-1@nexus-mail',
    }
    const { getByTestId } = open({
      draft: record({
        html: `<p><img src="cid:${image.cid}" data-cid="${image.cid}" alt="grafik.png"></p>`,
        inlineImages: [image],
      }),
    })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    const img = getByTestId('rich-editor').querySelector('img')
    expect(img?.getAttribute('src')).toBe(image.url)
    expect(img?.getAttribute('data-cid')).toBe(image.cid)
  })

  // A plain-text writer gets a plain-text message: no HTML part with markup
  // they did not ask for.
  it('sends no HTML from plain mode', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.change(getByTestId('composer-body'), { target: { value: 'düz metin' } })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({ html: '', text: 'düz metin' }),
      ),
    )
  })

  it('sends an embedded image as a cid part', async () => {
    const image = {
      path: '/tmp/grafik.png',
      cid: 'nx-1@nexus-mail',
      name: 'grafik.png',
      mimeType: 'image/png',
      url: '/compose-image/nx-1@nexus-mail',
    }
    pickInlineImage.mockResolvedValue(image)
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.click(getByTestId('composer-format'))
    fireEvent.click(getByTestId('rich-image'))
    await waitFor(() => expect(pickInlineImage).toHaveBeenCalled())

    // The editor's own insertion is covered where execCommand is mocked; here
    // the line it would produce is what matters.
    const editor = getByTestId('rich-editor')
    editor.innerHTML = `<p><img src="${image.url}" alt="${image.name}" data-cid="${image.cid}"></p>`
    fireEvent.input(editor)
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          html: expect.stringContaining(`cid:${image.cid}`),
          inlineImages: [expect.objectContaining({ path: image.path, cid: image.cid })],
        }),
      ),
    )
  })
})

describe('addressing', () => {
  // A message addressed to nobody is not a message, and a Send that fails for
  // that reason after a round trip is worse than one that was never offered.
  it('will not send with every address line empty', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect((getByTestId('composer-send') as HTMLButtonElement).disabled).toBe(true)

    fireEvent.change(getByTestId('composer-to'), { target: { value: '  ' } })
    expect((getByTestId('composer-send') as HTMLButtonElement).disabled).toBe(true)
  })

  // A blind copy alone is a legitimate message: an announcement to a list of
  // people who should not see each other.
  it('sends with only a blind copy', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-show-copies'))
    fireEvent.change(getByTestId('composer-bcc'), { target: { value: 'herkes@example.com' } })

    expect((getByTestId('composer-send') as HTMLButtonElement).disabled).toBe(false)
  })

  // The completion source is the account's own mail, not a network service:
  // people who have written before are the address book this build has.
  it('completes an address from someone who wrote before', async () => {
    suggestRecipients.mockResolvedValue([{ name: 'Alice', addr: 'alice@example.com' }])
    const { getByTestId, getByText, findByRole } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    const to = getByTestId('composer-to')
    fireEvent.focus(to)
    fireEvent.change(to, { target: { value: 'ali' } })
    fireEvent.click(await findByRole('option'))

    await waitFor(() => expect(suggestRecipients).toHaveBeenCalledWith(1, 'ali', 8))
    expect(getByText('Alice')).toBeTruthy()
    expect((to as HTMLInputElement).value).toBe('')
  })

  // Most messages have neither, and four empty lines is four lines of nothing
  // between the writer and what they came to write.
  it('hides the copy lines until they are asked for', async () => {
    const { getByTestId, queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect(queryByTestId('composer-cc')).toBeNull()
    expect(queryByTestId('composer-bcc')).toBeNull()

    fireEvent.click(getByTestId('composer-show-copies'))

    expect(getByTestId('composer-cc')).toBeTruthy()
    expect(getByTestId('composer-bcc')).toBeTruthy()
  })
})

describe('who it is from', () => {
  // One identity is not a choice, and a picker with one entry is a control
  // that asks a question with one answer.
  it('offers no picker when there is one address', async () => {
    const { queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect(queryByTestId('composer-from')).toBeNull()
  })

  it('offers the picker when there is more than one, opening on the default', async () => {
    identities.mockResolvedValue([
      identity({ id: 7, email: 'destek@example.com', from: 'Destek <destek@example.com>' }),
      identity({ id: 8, email: 'u@example.com', isDefault: false }),
    ])
    const { findByTestId } = open()

    const picker = (await findByTestId('composer-from')) as HTMLSelectElement
    // The backend returns them default first, so the first entry is the one to
    // open with.
    expect(picker.value).toBe('7')
  })

  it('sends as the chosen identity', async () => {
    identities.mockResolvedValue([
      identity({ id: 7 }),
      identity({ id: 8, email: 'destek@example.com', isDefault: false }),
    ])
    const { findByTestId, getByTestId } = open()

    fireEvent.change(await findByTestId('composer-from'), { target: { value: '8' } })
    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(expect.objectContaining({ identityId: 8 })),
    )
  })
})

describe('when it goes wrong', () => {
  // A send that failed and a composer that closed would be a message the
  // writer cannot recover.
  it('keeps the window and everything in it', async () => {
    sendMessage.mockRejectedValue(new Error('the To line is not a list of addresses'))
    const { getByTestId, onClose } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'bozuk' } })
    fireEvent.change(getByTestId('composer-body'), { target: { value: 'yazdığım metin' } })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() => expect(getByTestId('composer-error')).toBeTruthy())
    expect(getByTestId('composer-error').textContent).toContain('not a list of addresses')
    expect(onClose).not.toHaveBeenCalled()
    expect((getByTestId('composer-body') as HTMLTextAreaElement).value).toBe('yazdığım metin')
  })

  // And it can be tried again: a Send left disabled after a failure would
  // strand the message.
  it('can be sent again after a failure', async () => {
    sendMessage.mockRejectedValueOnce(new Error('nope'))
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.click(getByTestId('composer-send'))
    await waitFor(() => expect(getByTestId('composer-error')).toBeTruthy())

    expect((getByTestId('composer-send') as HTMLButtonElement).disabled).toBe(false)
    fireEvent.click(getByTestId('composer-send'))
    await waitFor(() => expect(sendMessage).toHaveBeenCalledTimes(2))
  })
})

describe('replying', () => {
  it('carries the threading headers and the subject through', async () => {
    const { getByTestId } = open({
      reply: {
        subject: 'Re: Konu',
        to: 'ilk@example.com',
        cc: '',
        inReplyTo: 'parent@example.com',
        references: ['root@example.com', 'parent@example.com'],
        quoted: '> özgün gövde',
      },
    })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect((getByTestId('composer-to') as HTMLInputElement).value).toBe('ilk@example.com')
    expect((getByTestId('composer-subject') as HTMLInputElement).value).toBe('Re: Konu')

    fireEvent.click(getByTestId('composer-send'))
    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({
          inReplyTo: 'parent@example.com',
          references: ['root@example.com', 'parent@example.com'],
        }),
      ),
    )
  })
})

describe('quoting', () => {
  const reply = {
    subject: 'Re: Konu',
    to: 'yazan@example.com',
    cc: 'biri@example.com',
    inReplyTo: 'parent@example.com',
    references: ['parent@example.com'],
    quoted: 'On 25 September 2026, Yazan wrote:\n> özgün gövde',
  }

  // Top-posting is what the rest of the world does and what a reader scanning
  // a thread expects: the quote is there to be referred to, not read first.
  it('puts the quote below an empty line and the cursor above it', async () => {
    const { getByTestId } = open({ reply })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    const body = getByTestId('composer-body') as HTMLTextAreaElement
    expect(body.value.startsWith('\n\n')).toBe(true)
    expect(body.value).toContain('> özgün gövde')
    expect(body.selectionStart).toBe(0)
  })

  // A Cc line with names on it that the writer cannot see is a message going
  // somewhere they did not check.
  it('shows the copy lines when a reply-all already filled them', async () => {
    const { getByTestId } = open({ reply })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect((getByTestId('composer-cc') as HTMLInputElement).value).toBe('biri@example.com')
  })

  it('sends the quote as part of the body', async () => {
    const { getByTestId } = open({ reply })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-send'))
    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({ text: expect.stringContaining('> özgün gövde') }),
      ),
    )
  })

  // A forward has nobody on it yet, and the heading has to say so rather than
  // calling itself a reply.
  it('calls itself a forward when there is nobody to reply to', async () => {
    const { getByText } = open({
      reply: { ...reply, to: '', cc: '', inReplyTo: '', references: [] },
    })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect(getByText('Forward')).toBeTruthy()
  })
})

describe('attaching files', () => {
  // Paths, not bytes. An eight-megabyte PDF serialised into an IPC message
  // would block this window for long enough to be seen.
  it('sends the paths of what was chosen', async () => {
    pickAttachments.mockResolvedValue([file()])
    const { getByTestId, findByText } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-attach'))
    await findByText('rapor.pdf')

    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(
        expect.objectContaining({ attachmentPaths: ['/tmp/rapor.pdf'] }),
      ),
    )
  })

  it('shows nothing until something is attached', async () => {
    const { queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect(queryByTestId('composer-attachments')).toBeNull()
  })

  // Choosing the same file twice is how somebody ends up mailing two copies of
  // one attachment.
  it('does not attach the same file twice', async () => {
    pickAttachments.mockResolvedValue([file()])
    const { getByTestId, findAllByText } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-attach'))
    await findAllByText('rapor.pdf')
    fireEvent.click(getByTestId('composer-attach'))

    await waitFor(async () => expect(await findAllByText('rapor.pdf')).toHaveLength(1))
  })

  it('can take a file back off', async () => {
    pickAttachments.mockResolvedValue([file()])
    const { getByTestId, getByLabelText, queryByTestId, findByText } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-attach'))
    await findByText('rapor.pdf')

    fireEvent.click(getByLabelText('Remove rapor.pdf'))

    await waitFor(() => expect(queryByTestId('composer-attachments')).toBeNull())
  })

  // Cancelling is an answer, not a failure, and an error message every time
  // somebody changes their mind would make the button unusable.
  it('says nothing when the dialog is closed', async () => {
    pickAttachments.mockResolvedValue([])
    const { getByTestId, queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-attach'))

    await waitFor(() => expect(pickAttachments).toHaveBeenCalled())
    expect(queryByTestId('composer-error')).toBeNull()
    expect(queryByTestId('composer-attachments')).toBeNull()
  })

  // A file that cannot be read is worth saying out loud: the alternative is a
  // paperclip button that sometimes does nothing.
  it('reports a dialog that failed', async () => {
    pickAttachments.mockRejectedValue(new Error('is a folder'))
    const { getByTestId, findByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-attach'))

    expect((await findByTestId('composer-error')).textContent).toContain('is a folder')
  })
})

describe('the attachment reminder', () => {
  const write = (getByTestId: (id: string) => HTMLElement) => {
    fireEvent.change(getByTestId('composer-to'), { target: { value: 'r@example.com' } })
    fireEvent.change(getByTestId('composer-body'), {
      target: { value: 'Please find the attached report.' },
    })
  }

  it('asks before sending a message that promises a file and carries none', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    write(getByTestId)
    fireEvent.click(getByTestId('composer-send'))

    expect(getByTestId('confirm-dialog')).toBeTruthy()
    expect(sendMessage).not.toHaveBeenCalled()
  })

  it('sends once the writer says the message is complete', async () => {
    const { getByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    write(getByTestId)
    fireEvent.click(getByTestId('composer-send'))
    fireEvent.click(getByTestId('confirm-accept'))

    await waitFor(() => expect(sendMessage).toHaveBeenCalled())
  })

  it('goes back to the message when the writer keeps editing', async () => {
    const { getByTestId, queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    write(getByTestId)
    fireEvent.click(getByTestId('composer-send'))
    fireEvent.click(getByTestId('confirm-cancel'))

    expect(queryByTestId('confirm-dialog')).toBeNull()
    expect(sendMessage).not.toHaveBeenCalled()
  })

  // With a file already attached there is nothing to warn about, however the
  // body reads.
  it('says nothing when something is attached', async () => {
    pickAttachments.mockResolvedValue([file()])
    const { getByTestId, findByText, queryByTestId } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-attach'))
    await findByText('rapor.pdf')
    write(getByTestId)
    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() => expect(sendMessage).toHaveBeenCalled())
    expect(queryByTestId('confirm-dialog')).toBeNull()
  })

  // A reply carries the original below the cursor. If the original mentions
  // its own attachment, every answer to it would otherwise ask the question.
  it('does not read the quoted original as a promise', async () => {
    const { getByTestId, queryByTestId } = open({
      reply: {
        subject: 'Re: rapor',
        to: 'r@example.com',
        cc: '',
        inReplyTo: '<x@example.com>',
        references: ['<x@example.com>'],
        quoted: 'Please see the attached file.',
      },
    })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() => expect(sendMessage).toHaveBeenCalled())
    expect(queryByTestId('confirm-dialog')).toBeNull()
  })
})

describe('not losing what was typed', () => {
  // Two seconds of nothing counts as having stopped typing.
  it('saves after a pause, not on every keystroke', async () => {
    vi.useFakeTimers()
    const { getByTestId } = open()
    await vi.waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-body'), { target: { value: 'b' } })
    fireEvent.change(getByTestId('composer-body'), { target: { value: 'bi' } })
    fireEvent.change(getByTestId('composer-body'), { target: { value: 'bir' } })
    expect(saveDraft).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(2100)
    expect(saveDraft).toHaveBeenCalledTimes(1)
    expect(saveDraft).toHaveBeenCalledWith(expect.objectContaining({ body: 'bir' }))
  })

  // One row, not one per pause. The id has to come back into the next save.
  it('writes into the same draft on the second pause', async () => {
    vi.useFakeTimers()
    const { getByTestId } = open()
    await vi.waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-body'), { target: { value: 'ilk' } })
    await vi.advanceTimersByTimeAsync(2100)
    expect(saveDraft).toHaveBeenLastCalledWith(expect.objectContaining({ id: 0 }))

    fireEvent.change(getByTestId('composer-body'), { target: { value: 'ikinci' } })
    await vi.advanceTimersByTimeAsync(2100)
    expect(saveDraft).toHaveBeenLastCalledWith(expect.objectContaining({ id: 7 }))
  })

  // Opening a composer, thinking better of it and closing it should leave
  // nothing behind. A Drafts folder full of blank messages is one nobody reads.
  it('does not save an empty composer', async () => {
    vi.useFakeTimers()
    open()
    await vi.waitFor(() => expect(identities).toHaveBeenCalled())

    await vi.advanceTimersByTimeAsync(5000)
    expect(saveDraft).not.toHaveBeenCalled()
  })

  // The pause is not enough on its own: typing a sentence and closing the
  // window straight away would clear the pending timer and lose it, which is
  // the exact thing this mechanism exists to prevent.
  it('saves on the way out, even before the pause is up', async () => {
    const { getByTestId, unmount } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.change(getByTestId('composer-body'), { target: { value: 'aceleyle' } })
    unmount()

    await waitFor(() =>
      expect(saveDraft).toHaveBeenCalledWith(expect.objectContaining({ body: 'aceleyle' })),
    )
  })

  // And a composer that was never typed into leaves nothing behind on the way
  // out either.
  it('saves nothing on the way out of an empty composer', async () => {
    const { unmount } = open()
    await waitFor(() => expect(identities).toHaveBeenCalled())

    unmount()

    expect(saveDraft).not.toHaveBeenCalled()
  })

  // The address line reopens as a finished pill plus the unfinished tail, not
  // as one long string: the pills are the same line the backend will receive,
  // only laid out the way the writer left it.
  it('comes back with everything as it was typed', async () => {
    const { getByTestId, getByText } = open({ draft: record() })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect(getByText('ali@example.com')).toBeTruthy()
    expect((getByTestId('composer-to') as HTMLInputElement).value).toBe('bir yar')
    expect((getByTestId('composer-subject') as HTMLInputElement).value).toBe('Yarim konu')
    expect((getByTestId('composer-body') as HTMLTextAreaElement).value).toBe('burada kalmis')
  })

  // A blind copy the writer cannot see is a message going somewhere they did
  // not check, which is as true on reopening as it is on reply-all.
  it('shows the copy lines when the draft has them', async () => {
    const { getByTestId } = open({ draft: record({ bcc: 'gizli@example.com' }) })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    expect((getByTestId('composer-bcc') as HTMLInputElement).value).toBe('gizli@example.com')
  })

  it('tells the backend which draft the sent message came from', async () => {
    const { getByTestId } = open({ draft: record() })
    await waitFor(() => expect(identities).toHaveBeenCalled())

    fireEvent.click(getByTestId('composer-send'))

    await waitFor(() =>
      expect(sendMessage).toHaveBeenCalledWith(expect.objectContaining({ draftId: 7 })),
    )
  })

  // A composer that claims to be saved when it is not is worse than one that
  // says nothing: that claim is what people close the window on.
  it('says nothing about saving until a save has happened', async () => {
    vi.useFakeTimers()
    saveDraft.mockRejectedValue(new Error('disk full'))
    const { getByTestId } = open()
    await vi.waitFor(() => expect(identities).toHaveBeenCalled())

    expect(getByTestId('composer-saved').textContent).toBe('')

    fireEvent.change(getByTestId('composer-body'), { target: { value: 'metin' } })
    await vi.advanceTimersByTimeAsync(2100)

    expect(getByTestId('composer-saved').textContent).toBe('')
    // And it is not shouted about either: the person is mid-sentence.
    expect(getByTestId('composer-body')).toBeTruthy()
  })
})
