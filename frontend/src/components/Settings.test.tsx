import { fireEvent, render, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Settings } from './Settings'
import { useMailStore } from '../store/useMailStore'
import type { AppSettings } from '../lib/api'

const settings = vi.fn()
const updateSettings = vi.fn()

vi.mock('../lib/api', () => ({
  settings: () => settings(),
  updateSettings: (next: AppSettings) => updateSettings(next),
}))

const stored = (over: Partial<AppSettings> = {}): AppSettings => ({
  googleClientId: '',
  microsoftClientId: '',
  oauthRedirectPort: 0,
  retentionDays: 365,
  retentionMaxMessages: 25000,
  notificationPreview: true,
  undoWindowSeconds: 5,
  startAtLogin: false,
  startAtLoginAvailable: true,
  ...over,
})

beforeEach(() => {
  useMailStore.getState().reset()
  settings.mockReset()
  settings.mockResolvedValue(stored())
  updateSettings.mockReset()
  updateSettings.mockResolvedValue(undefined)
  try {
    localStorage.clear()
  } catch {
    // Storage is unavailable in this environment; the prefs fall back to
    // defaults, which is what these tests assume anyway.
  }
})

describe('starting with the machine', () => {
  it('offers the switch when the machine can be asked', async () => {
    const { getByTestId } = render(<Settings onClose={() => {}} />)

    await waitFor(() => expect(getByTestId('start-at-login-on')).toBeTruthy())
    expect((getByTestId('start-at-login-on') as HTMLButtonElement).disabled).toBe(false)
  })

  it('saves the change to the backend', async () => {
    const { getByTestId } = render(<Settings onClose={() => {}} />)
    await waitFor(() => expect(getByTestId('start-at-login-on')).toBeTruthy())

    fireEvent.click(getByTestId('start-at-login-on'))

    await waitFor(() =>
      expect(updateSettings).toHaveBeenCalledWith(
        expect.objectContaining({ startAtLogin: true }),
      ),
    )
  })

  // Off and "we could not ask" are different answers. A live switch reading
  // Off would be claiming a state nobody established, and clicking it would
  // fail for a reason the screen never mentioned.
  it('disables the switch rather than showing it off when it cannot be read', async () => {
    settings.mockResolvedValue(stored({ startAtLoginAvailable: false }))
    const { getByTestId } = render(<Settings onClose={() => {}} />)

    await waitFor(() => expect(getByTestId('start-at-login-on')).toBeTruthy())
    expect((getByTestId('start-at-login-on') as HTMLButtonElement).disabled).toBe(true)
    expect((getByTestId('start-at-login-off') as HTMLButtonElement).disabled).toBe(true)
  })

  it('says why it is unavailable rather than leaving a dead control', async () => {
    settings.mockResolvedValue(stored({ startAtLoginAvailable: false }))
    const { findByText } = render(<Settings onClose={() => {}} />)

    expect(await findByText(/cannot register itself to start at login/i)).toBeTruthy()
  })
})

describe('settings that live in the window', () => {
  // They change what is on screen, so they apply as they are chosen. Waiting
  // for a Save would mean the reader cannot see what they picked.
  it('applies the theme immediately, with nothing sent to the backend', async () => {
    const { getByTestId } = render(<Settings onClose={() => {}} />)

    fireEvent.click(getByTestId('theme-dark'))

    expect(useMailStore.getState().themeChoice).toBe('dark')
    expect(updateSettings).not.toHaveBeenCalled()
  })

  it('applies conversation grouping immediately', () => {
    const { getByTestId } = render(<Settings onClose={() => {}} />)

    fireEvent.click(getByTestId('threaded-on'))

    expect(useMailStore.getState().threaded).toBe(true)
  })

  // Three answers because people genuinely want different ones, and the
  // default has to stay what the app did before the setting existed.
  it('offers three answers for marking read, starting at the old behaviour', () => {
    const { getByTestId } = render(<Settings onClose={() => {}} />)

    expect(getByTestId('mark-read-open').getAttribute('aria-checked')).toBe('true')

    fireEvent.click(getByTestId('mark-read-never'))
    expect(getByTestId('mark-read-never').getAttribute('aria-checked')).toBe('true')
  })
})

describe('settings that live in the file', () => {
  it('shows what the backend holds', async () => {
    settings.mockResolvedValue(stored({ microsoftClientId: 'abc-123', undoWindowSeconds: 12 }))
    const { findByTestId } = render(<Settings onClose={() => {}} />)

    expect(((await findByTestId('microsoftClientId')) as HTMLInputElement).value).toBe('abc-123')
    expect(((await findByTestId('undoWindowSeconds')) as HTMLInputElement).value).toBe('12')
  })

  it('saves a choice as it is made', async () => {
    const { findByTestId } = render(<Settings onClose={() => {}} />)

    fireEvent.click(await findByTestId('notification-preview-off'))

    await waitFor(() =>
      expect(updateSettings).toHaveBeenCalledWith(
        expect.objectContaining({ notificationPreview: false }),
      ),
    )
  })

  // A partially typed number is not a setting: writing "1" on the way to "10"
  // would briefly mean something quite different.
  it('saves a number only once the field is left', async () => {
    const { findByTestId } = render(<Settings onClose={() => {}} />)
    const field = (await findByTestId('undoWindowSeconds')) as HTMLInputElement

    fireEvent.change(field, { target: { value: '2' } })
    expect(updateSettings).not.toHaveBeenCalled()

    fireEvent.blur(field)
    await waitFor(() =>
      expect(updateSettings).toHaveBeenCalledWith(
        expect.objectContaining({ undoWindowSeconds: 2 }),
      ),
    )
  })

  it('does not save a field that was not changed', async () => {
    const { findByTestId } = render(<Settings onClose={() => {}} />)
    const field = (await findByTestId('retentionDays')) as HTMLInputElement

    fireEvent.blur(field)

    expect(updateSettings).not.toHaveBeenCalled()
  })

  // A screen that showed a value the backend refused would be lying about
  // what the app is doing.
  it('puts the old value back when the backend refuses', async () => {
    updateSettings.mockRejectedValue(new Error('undoWindowSeconds cannot be negative'))
    const { findByTestId, getByTestId } = render(<Settings onClose={() => {}} />)

    fireEvent.click(await findByTestId('notification-preview-off'))

    await waitFor(() => expect(getByTestId('settings-error')).toBeTruthy())
    expect(getByTestId('settings-error').textContent).toContain('cannot be negative')
    expect(getByTestId('notification-preview-on').getAttribute('aria-checked')).toBe('true')
  })

  // The window has to stay usable when the backend cannot be reached, because
  // half of these settings do not need it.
  it('still shows the window settings when the backend fails', async () => {
    settings.mockRejectedValue(new Error('nope'))
    const { findByTestId, queryByTestId } = render(<Settings onClose={() => {}} />)

    expect(await findByTestId('settings-error')).toBeTruthy()
    expect(queryByTestId('theme-dark')).toBeTruthy()
    expect(queryByTestId('undoWindowSeconds')).toBeNull()
  })
})

describe('leaving', () => {
  it('goes back to the mail', () => {
    const onClose = vi.fn()
    const { getByTestId } = render(<Settings onClose={onClose} />)

    fireEvent.click(getByTestId('settings-close'))

    expect(onClose).toHaveBeenCalledOnce()
  })
})
