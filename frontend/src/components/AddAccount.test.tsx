import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AddAccount } from './AddAccount'

const addPasswordAccount = vi.fn()
const addOAuthAccount = vi.fn()
const syncAccount = vi.fn()

vi.mock('../lib/api', () => ({
  addPasswordAccount: (...args: unknown[]) => addPasswordAccount(...args),
  addOAuthAccount: (...args: unknown[]) => addOAuthAccount(...args),
  syncAccount: (id: number) => syncAccount(id),
}))

beforeEach(() => {
  addPasswordAccount.mockReset()
  addPasswordAccount.mockResolvedValue({ id: 1 })
  addOAuthAccount.mockReset()
  syncAccount.mockReset()
  syncAccount.mockResolvedValue(undefined)
})

const fillServerAccount = (getByTestId: (id: string) => HTMLElement) => {
  fireEvent.click(getByTestId('mode-password'))
  fireEvent.change(getByTestId('email'), { target: { value: 'bt@sirket.local' } })
  fireEvent.change(getByTestId('imap-host'), { target: { value: 'mail.sirket.local' } })
  fireEvent.change(getByTestId('password'), { target: { value: 'gizli' } })
}

describe('connection security', () => {
  // On-premises Exchange publishes IMAP on 143 and will not take a password
  // until the connection has been upgraded. Without this choice there is no
  // way to reach it.
  it('sends the chosen encryption with the account', async () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fillServerAccount(getByTestId)
    fireEvent.click(getByTestId('security-starttls'))
    fireEvent.click(getByTestId('submit-account'))

    await waitFor(() => expect(addPasswordAccount).toHaveBeenCalled())
    const args = addPasswordAccount.mock.calls[0]
    expect(args[4]).toBe('starttls')
  })

  it('defaults to implicit TLS', async () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fillServerAccount(getByTestId)
    fireEvent.click(getByTestId('submit-account'))

    await waitFor(() => expect(addPasswordAccount).toHaveBeenCalled())
    expect(addPasswordAccount.mock.calls[0][4]).toBe('tls')
  })

  // Somebody told "use STARTTLS" and nothing else should not have to know that
  // it means 143.
  it('moves the port to match the encryption', () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-password'))
    const port = getByTestId('imap-port') as HTMLInputElement
    expect(port.value).toBe('993')

    fireEvent.click(getByTestId('security-starttls'))
    expect(port.value).toBe('143')

    fireEvent.click(getByTestId('security-tls'))
    expect(port.value).toBe('993')
  })

  // Someone who typed a non-standard port knows something this code does not,
  // and having it overwritten under them is worse than any default.
  it('leaves a port the user chose alone', () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-password'))
    const port = getByTestId('imap-port') as HTMLInputElement
    fireEvent.change(port, { target: { value: '10143' } })

    fireEvent.click(getByTestId('security-starttls'))
    expect(port.value).toBe('10143')
  })

  // There is no third option. A password sent in the clear is a password given
  // away, and the absence of the choice is what makes that impossible rather
  // than merely discouraged.
  it('offers no unencrypted option', () => {
    const { getByTestId, queryByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-password'))
    expect(queryByTestId('security-none')).toBeNull()
    expect(queryByTestId('security-plain')).toBeNull()
  })
})
