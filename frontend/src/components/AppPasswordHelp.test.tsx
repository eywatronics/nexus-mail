import { fireEvent, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AppPasswordHelp } from './AppPasswordHelp'

const openExternal = vi.fn()

vi.mock('../lib/external', () => ({
  openExternal: (...args: unknown[]) => openExternal(...args),
}))

beforeEach(() => {
  openExternal.mockReset()
  openExternal.mockResolvedValue(undefined)
})

const show = (
  email: string,
  onUseMicrosoftSignIn = vi.fn(),
  assumeGoogle = false,
) =>
  render(
    <AppPasswordHelp
      email={email}
      assumeGoogle={assumeGoogle}
      onUseMicrosoftSignIn={onUseMicrosoftSignIn}
    />,
  )

describe('which providers need an app password', () => {
  // The backend only knows preset servers for these domains, so the window
  // only has something useful to say where a blank host will also work.
  it('knows Gmail and Googlemail', () => {
    for (const email of ['a@gmail.com', 'a@googlemail.com']) {
      const { getByTestId, unmount } = show(email)
      expect(getByTestId('app-password-google')).toBeTruthy()
      unmount()
    }
  })

  it('knows the Microsoft consumer domains', () => {
    for (const email of [
      'a@outlook.com',
      'a@hotmail.com',
      'a@live.com',
      'a@msn.com',
      'a@office365.com',
    ]) {
      const { getByTestId, unmount } = show(email)
      expect(getByTestId('app-password-microsoft')).toBeTruthy()
      unmount()
    }
  })

  it('knows iCloud and Yahoo', () => {
    const icloud = show('a@icloud.com')
    expect(icloud.getByTestId('app-password-apple')).toBeTruthy()
    icloud.unmount()

    const yahoo = show('a@yahoo.com')
    expect(yahoo.getByTestId('app-password-yahoo')).toBeTruthy()
    yahoo.unmount()
  })

  // A Workspace account lives on its own domain and looks like any other
  // address. The escape hatch from the Gmail sign-in is the one thing that
  // tells the window it is still Google.
  it('takes the Gmail hint for an address it cannot recognise', () => {
    const { getByTestId } = show('ali@sirket.com', vi.fn(), true)
    expect(getByTestId('app-password-google')).toBeTruthy()
  })

  it('still believes the address when the address says Microsoft', () => {
    const { getByTestId, queryByTestId } = show('ali@outlook.com', vi.fn(), true)
    expect(getByTestId('app-password-microsoft')).toBeTruthy()
    expect(queryByTestId('app-password-google')).toBeNull()
  })

  // A corporate address is the common case for the password path, and telling
  // it about Gmail would be noise at the exact moment somebody is typing a
  // server name their IT gave them.
  it('stays quiet for an unknown domain', () => {
    const { queryByTestId } = show('bt@sirket.local')
    expect(queryByTestId('app-password-google')).toBeNull()
    expect(queryByTestId('app-password-microsoft')).toBeNull()
    expect(queryByTestId('app-password-apple')).toBeNull()
    expect(queryByTestId('app-password-yahoo')).toBeNull()
  })
})

describe('the Google instructions', () => {
  it('opens the two Google pages in the browser', () => {
    const { getByTestId } = show('someone@gmail.com')

    fireEvent.click(getByTestId('open-google-2fa'))
    expect(openExternal).toHaveBeenCalledWith('https://myaccount.google.com/security')

    fireEvent.click(getByTestId('open-google-app-passwords'))
    expect(openExternal).toHaveBeenCalledWith('https://myaccount.google.com/apppasswords')
  })

  it('says that a workspace administrator can turn the route off', () => {
    const { getByText } = show('someone@gmail.com')
    expect(getByText(/administrator may have turned app passwords off/i)).toBeTruthy()
  })
})

describe('the Microsoft correction', () => {
  // App passwords did work for Outlook.com once. Sending somebody to generate
  // one today is sending them to a page whose result this client cannot use.
  it('says a password cannot connect the account', () => {
    const { getByText } = show('someone@outlook.com')
    expect(getByText(/no longer accepts passwords/i)).toBeTruthy()
    expect(getByText(/cannot connect this account/i)).toBeTruthy()
  })

  it('offers the sign-in that can', () => {
    const onUseMicrosoftSignIn = vi.fn()
    const { getByTestId } = show('someone@outlook.com', onUseMicrosoftSignIn)

    fireEvent.click(getByTestId('use-microsoft-signin'))
    expect(onUseMicrosoftSignIn).toHaveBeenCalledTimes(1)
  })
})
