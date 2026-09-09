import { useState } from 'react'
import { addOAuthAccount, addPasswordAccount, syncAccount } from '../lib/api'

type Mode = 'microsoft' | 'google' | 'password'

const MODES: Array<[Mode, string, string]> = [
  ['microsoft', 'Microsoft 365 or Outlook.com', 'Signs in through your browser.'],
  ['google', 'Gmail or Google Workspace', 'Signs in through your browser.'],
  ['password', 'Other IMAP server', 'Password or app password.'],
]

interface AddAccountProps {
  onDone: () => void
  onCancel?: () => void
}

export function AddAccount({ onDone, onCancel }: AddAccountProps) {
  const [mode, setMode] = useState<Mode>('microsoft')
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [imapHost, setImapHost] = useState('')
  const [imapPort, setImapPort] = useState(993)
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      const account =
        mode === 'password'
          ? await addPasswordAccount(email, displayName, imapHost, imapPort, '', 0, password)
          : await addOAuthAccount(email, displayName, mode)

      // Drop the password from component state the moment it is stored, so it
      // does not sit in a React tree for the rest of the session.
      setPassword('')

      await syncAccount(account.id)
      onDone()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const inputClass =
    'rounded border border-neutral-300 px-3 py-2 text-sm dark:border-neutral-700 dark:bg-neutral-900'

  return (
    <div className="mx-auto flex h-full max-w-md flex-col justify-center p-6">
      <h1 className="mb-1 text-lg font-semibold">Add an account</h1>
      <p className="mb-5 text-sm text-neutral-500">
        Your mail stays on this machine. Credentials go to your operating system’s
        keyring, never to a file.
      </p>

      <fieldset className="mb-4 flex flex-col gap-2">
        <legend className="sr-only">Account type</legend>
        {MODES.map(([value, label, hint]) => (
          <label key={value} className="flex items-start gap-2 text-sm">
            <input
              type="radio"
              name="mode"
              value={value}
              checked={mode === value}
              onChange={() => setMode(value)}
              className="mt-1"
            />
            <span>
              {label}
              <span className="block text-xs text-neutral-500">{hint}</span>
            </span>
          </label>
        ))}
      </fieldset>

      <div className="flex flex-col gap-3">
        <input
          type="email"
          aria-label="Email address"
          placeholder="you@example.com"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          className={inputClass}
        />
        <input
          type="text"
          aria-label="Display name"
          placeholder="Display name"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
          className={inputClass}
        />

        {mode === 'password' && (
          <>
            <input
              type="text"
              aria-label="IMAP host"
              placeholder="IMAP host (blank for known providers)"
              value={imapHost}
              onChange={(e) => setImapHost(e.target.value)}
              className={inputClass}
            />
            <input
              type="number"
              aria-label="IMAP port"
              value={imapPort}
              onChange={(e) => setImapPort(Number(e.target.value))}
              className={inputClass}
            />
            <input
              type="password"
              aria-label="Password"
              placeholder="Password or app password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="off"
              className={inputClass}
            />
          </>
        )}

        {mode !== 'password' && (
          <p className="text-xs text-neutral-500">
            Your browser will open so you can sign in. Nexus Mail never sees your
            password.
            {mode === 'google' &&
              ' Gmail also needs an OAuth client ID of your own — see docs/oauth-setup.md.'}
          </p>
        )}

        {error && (
          <p role="alert" className="text-sm text-red-600">
            {error}
          </p>
        )}

        <div className="flex gap-2">
          <button
            type="button"
            onClick={submit}
            disabled={busy || email === ''}
            className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
          >
            {busy ? 'Connecting…' : 'Add account'}
          </button>
          {onCancel && (
            <button
              type="button"
              onClick={onCancel}
              disabled={busy}
              className="rounded border border-neutral-300 px-4 py-2 text-sm dark:border-neutral-700"
            >
              Cancel
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
