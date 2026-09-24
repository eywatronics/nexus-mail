import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Composer } from './Composer'
import type { Identity } from '../lib/api'

const identities = vi.fn()
const sendMessage = vi.fn()

vi.mock('../lib/api', () => ({
  identities: (accountId: number) => identities(accountId),
  sendMessage: (draft: unknown) => sendMessage(draft),
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
        inReplyTo: 'parent@example.com',
        references: ['root@example.com', 'parent@example.com'],
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
