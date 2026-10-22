import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AddAccount } from './AddAccount'

const addPasswordAccount = vi.fn()
const addOAuthAccount = vi.fn()
const syncAccount = vi.fn()
const discoverServer = vi.fn()

vi.mock('../lib/api', () => ({
  addPasswordAccount: (...args: unknown[]) => addPasswordAccount(...args),
  addOAuthAccount: (...args: unknown[]) => addOAuthAccount(...args),
  syncAccount: (id: number) => syncAccount(id),
  discoverServer: (...args: unknown[]) => discoverServer(...args),
}))

// The guidance opens provider pages through the desktop shell. A test has no
// shell, and the click itself is covered where the button lives.
vi.mock('../lib/external', () => ({ openExternal: vi.fn() }))

beforeEach(() => {
  addPasswordAccount.mockReset()
  addPasswordAccount.mockResolvedValue({ id: 1 })
  addOAuthAccount.mockReset()
  syncAccount.mockReset()
  syncAccount.mockResolvedValue(undefined)
  discoverServer.mockReset()
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

describe('the app password route', () => {
  // The whole reason this screen exists for a Gmail user without a Cloud
  // project. It has to be one step from the sign-in they cannot finish, and it
  // has to land on a form that can actually connect.
  it('is one step from the Gmail sign-in and fills the server in', () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-google'))
    fireEvent.change(getByTestId('email'), { target: { value: 'someone@gmail.com' } })
    fireEvent.click(getByTestId('gmail-use-app-password'))

    expect((getByTestId('mode-password') as HTMLInputElement).checked).toBe(true)
    expect((getByTestId('email') as HTMLInputElement).value).toBe('someone@gmail.com')
    expect((getByTestId('imap-host') as HTMLInputElement).value).toBe('imap.gmail.com')
    expect((getByTestId('imap-port') as HTMLInputElement).value).toBe('993')
    // One without the other is an account that can receive and not send.
    expect((getByTestId('smtp-host') as HTMLInputElement).value).toBe('smtp.gmail.com')
    expect((getByTestId('smtp-port') as HTMLInputElement).value).toBe('587')
    expect(getByTestId('app-password-google')).toBeTruthy()
  })

  it('explains what Google needs once a Gmail address is typed', () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-password'))
    fireEvent.change(getByTestId('email'), { target: { value: 'someone@gmail.com' } })

    expect(getByTestId('app-password-google')).toBeTruthy()
  })

  // A Workspace account on a company domain has no gmail.com to recognise,
  // but it still needs the same app password and the same two Google pages.
  it('keeps the Google instructions for a Workspace address', () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-google'))
    fireEvent.change(getByTestId('email'), { target: { value: 'ali@sirket.com' } })
    fireEvent.click(getByTestId('gmail-use-app-password'))

    expect(getByTestId('app-password-google')).toBeTruthy()
    expect((getByTestId('imap-host') as HTMLInputElement).value).toBe('imap.gmail.com')
  })

  // Microsoft stopped accepting app passwords for IMAP. Leaving the form on
  // the password path would be a button that cannot work, however well filled
  // in it is.
  it('turns a Microsoft address away from the password path', () => {
    const { getByTestId, queryByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-password'))
    fireEvent.change(getByTestId('email'), { target: { value: 'someone@outlook.com' } })
    expect(getByTestId('app-password-microsoft')).toBeTruthy()

    fireEvent.click(getByTestId('use-microsoft-signin'))

    expect((getByTestId('mode-microsoft') as HTMLInputElement).checked).toBe(true)
    expect((getByTestId('email') as HTMLInputElement).value).toBe('someone@outlook.com')
    expect(queryByTestId('app-password-microsoft')).toBeNull()
  })

  // A corporate Exchange server is the ordinary case for this form, and it
  // must not be told that it needs a Gmail app password.
  it('says nothing to an address it has no advice for', () => {
    const { getByTestId, queryByTestId } = render(<AddAccount onDone={() => {}} />)

    fireEvent.click(getByTestId('mode-password'))
    fireEvent.change(getByTestId('email'), { target: { value: 'bt@sirket.local' } })

    expect(queryByTestId('app-password-google')).toBeNull()
    expect(queryByTestId('app-password-microsoft')).toBeNull()
  })
})

describe('finding the servers', () => {
  const found = {
    imapHost: 'imap.example.com',
    imapPort: 993,
    security: 'tls',
    smtpHost: 'smtp.example.com',
    smtpPort: 587,
    source: 'srv',
  }

  const start = (getByTestId: (id: string) => HTMLElement) => {
    fireEvent.click(getByTestId('mode-password'))
    fireEvent.change(getByTestId('email'), { target: { value: 'ali@example.com' } })
  }

  it('fills IMAP, SMTP and ports from the lookup', async () => {
    discoverServer.mockResolvedValue(found)
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    start(getByTestId)
    fireEvent.click(getByTestId('discover-server'))

    await waitFor(() =>
      expect((getByTestId('imap-host') as HTMLInputElement).value).toBe(
        'imap.example.com',
      ),
    )
    expect((getByTestId('imap-port') as HTMLInputElement).value).toBe('993')
    expect((getByTestId('smtp-host') as HTMLInputElement).value).toBe(
      'smtp.example.com',
    )
    expect((getByTestId('smtp-port') as HTMLInputElement).value).toBe('587')
    expect((getByTestId('security-tls') as HTMLInputElement).checked).toBe(true)
    expect(getByTestId('discovery-note').textContent).toContain('published DNS')
  })

  it('carries a STARTTLS answer into the form', async () => {
    discoverServer.mockResolvedValue({
      ...found,
      imapHost: 'imap.example.com',
      imapPort: 143,
      security: 'starttls',
      source: 'guess',
    })
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    start(getByTestId)
    fireEvent.click(getByTestId('discover-server'))

    await waitFor(() =>
      expect((getByTestId('imap-port') as HTMLInputElement).value).toBe('143'),
    )
    expect((getByTestId('security-starttls') as HTMLInputElement).checked).toBe(
      true,
    )
    expect(getByTestId('discovery-note').textContent).toContain('Check them')
  })

  it('reports a lookup that found nothing', async () => {
    discoverServer.mockRejectedValue(new Error('no IMAP server found for example.com'))
    const { getByTestId, getByRole } = render(<AddAccount onDone={() => {}} />)

    start(getByTestId)
    fireEvent.click(getByTestId('discover-server'))

    await waitFor(() =>
      expect(getByRole('alert').textContent).toContain('no IMAP server found'),
    )
  })

  // Finding the servers is only useful if what was found is what gets used.
  it('sends the SMTP settings with the account', async () => {
    const { getByTestId } = render(<AddAccount onDone={() => {}} />)

    start(getByTestId)
    fireEvent.change(getByTestId('imap-host'), { target: { value: 'imap.example.com' } })
    fireEvent.change(getByTestId('smtp-host'), { target: { value: 'smtp.example.com' } })
    fireEvent.change(getByTestId('smtp-port'), { target: { value: '2525' } })
    fireEvent.change(getByTestId('password'), { target: { value: 'gizli' } })
    fireEvent.click(getByTestId('submit-account'))

    await waitFor(() => expect(addPasswordAccount).toHaveBeenCalled())
    const args = addPasswordAccount.mock.calls[0]
    expect(args[5]).toBe('smtp.example.com')
    expect(args[6]).toBe(2525)
  })
})

describe('settings before an account exists', () => {
  // The client ID the sign-in pane asks for lives in Settings, and Settings
  // used to be reachable only after an account had been added. On a fresh
  // install that made the Microsoft path a dead end.
  it('offers the screen that holds the client ID', () => {
    const onOpenSettings = vi.fn()
    const { getByTestId } = render(
      <AddAccount onDone={() => {}} onOpenSettings={onOpenSettings} />,
    )

    fireEvent.click(getByTestId('open-oauth-settings'))
    expect(onOpenSettings).toHaveBeenCalledTimes(1)
  })

  it('shows no dead link when the shell has no settings screen', () => {
    const { queryByTestId } = render(<AddAccount onDone={() => {}} />)
    expect(queryByTestId('open-oauth-settings')).toBeNull()
  })
})
