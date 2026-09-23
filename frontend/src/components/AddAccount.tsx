import {
  ArrowLeft,
  CircleNotch,
  EyeSlash,
  HardDrives,
  Key,
  MicrosoftOutlookLogo,
  GoogleLogo,
  Envelope,
  Warning,
} from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { useState } from 'react'
import { addOAuthAccount, addPasswordAccount, syncAccount } from '../lib/api'
import {
  BUTTON_PRIMARY,
  BUTTON_SECONDARY,
  ICON,
  INPUT,
  RADIUS,
  SURFACE,
  TEXT,
} from '../lib/ui'

type Mode = 'microsoft' | 'google' | 'password'

const MODES: Array<{ value: Mode; label: string; hint: string; icon: Icon }> = [
  {
    value: 'microsoft',
    label: 'Microsoft 365 or Outlook.com',
    hint: 'Opens your browser to sign in',
    icon: MicrosoftOutlookLogo,
  },
  {
    value: 'google',
    label: 'Gmail or Google Workspace',
    hint: 'Opens your browser to sign in',
    icon: GoogleLogo,
  },
  {
    value: 'password',
    label: 'Other IMAP server',
    hint: 'Password or app password',
    icon: Envelope,
  },
]

type Security = 'tls' | 'starttls'

/**
 * Both options encrypt. There is no third one: a password sent in the clear is
 * a password given away, and offering that choice would make a
 * misconfiguration silent instead of impossible.
 *
 * STARTTLS is here because of on-premises Exchange. Its IMAP4 service defaults
 * to LoginType SecureLogin, which will not accept a password until the
 * connection has been upgraded, and plenty of deployments publish only 143.
 */
const SECURITIES: Array<{ value: Security; label: string; hint: string }> = [
  { value: 'tls', label: 'SSL/TLS', hint: 'Port 993' },
  { value: 'starttls', label: 'STARTTLS', hint: 'Port 143' },
]

const DEFAULT_PORTS: Record<Security, number> = { tls: 993, starttls: 143 }

const PROMISES: Array<{ icon: Icon; title: string; body: string }> = [
  {
    icon: HardDrives,
    title: 'Your mail stays on this machine',
    body: 'Everything renders from a local database, so reading works with the network off.',
  },
  {
    icon: EyeSlash,
    title: 'Trackers blocked by default',
    body: 'Remote images load only when you ask, and then through this app rather than your address.',
  },
  {
    icon: Key,
    title: 'Credentials in your OS keyring',
    body: 'Never in the database, never in a config file.',
  },
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
  const [security, setSecurity] = useState<Security>('tls')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    setBusy(true)
    setError('')
    try {
      const account =
        mode === 'password'
          ? await addPasswordAccount(
              email,
              displayName,
              imapHost,
              imapPort,
              security,
              '',
              0,
              password,
            )
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

  return (
    <div className={`h-full overflow-y-auto ${SURFACE.page}`}>
      {/* Split rather than a centred stack: a desktop window has width, and a
          narrow column down the middle of a 1280px app wastes it. The left
          side answers "what is this", the right side gets on with the job. */}
      <div className="mx-auto grid min-h-full max-w-5xl grid-cols-1 items-center gap-12 px-8 py-16 lg:grid-cols-[minmax(0,1fr)_minmax(0,26rem)] lg:gap-16">
        <section className="max-w-md">
          <h1 className={`text-3xl font-semibold tracking-tight ${TEXT.primary}`}>
            Nexus Mail
          </h1>
          <p className={`mt-3 max-w-[46ch] text-base leading-relaxed ${TEXT.secondary}`}>
            A mail client that keeps your inbox on your own disk and your reading to
            yourself.
          </p>

          <ul className="mt-10 flex flex-col gap-6">
            {PROMISES.map(({ icon: PromiseIcon, title, body }) => (
              <li key={title} className="flex gap-3">
                <PromiseIcon
                  size={20}
                  weight="light"
                  aria-hidden
                  className="mt-0.5 shrink-0 text-[var(--color-accent)]"
                />
                <div>
                  <h2 className={`text-sm font-medium ${TEXT.primary}`}>{title}</h2>
                  <p className={`mt-1 max-w-[42ch] text-sm leading-relaxed ${TEXT.secondary}`}>
                    {body}
                  </p>
                </div>
              </li>
            ))}
          </ul>
        </section>

        <section
          aria-labelledby="add-account-heading"
          className={`border p-6 ${SURFACE.divider} ${RADIUS} ${SURFACE.panel}`}
        >
          <h2 id="add-account-heading" className={`text-sm font-medium ${TEXT.primary}`}>
            Add an account
          </h2>

          <fieldset className="mt-4">
            <legend className="sr-only">Account type</legend>
            <div className="flex flex-col gap-1.5">
              {MODES.map(({ value, label, hint, icon: ModeIcon }) => {
                const active = mode === value
                return (
                  <label
                    key={value}
                    className={[
                      RADIUS,
                      'flex cursor-pointer items-center gap-3 border p-2.5 transition-colors',
                      active
                        ? 'border-[var(--color-accent)] bg-[var(--color-surface-selected)] dark:bg-[var(--color-surface-selected-dark)]'
                        : `${SURFACE.divider} hover:bg-neutral-100 dark:hover:bg-neutral-800`,
                    ].join(' ')}
                  >
                    <input
                      type="radio"
                      name="mode"
                      value={value}
                      checked={active}
                      data-testid={`mode-${value}`}
                      onChange={() => setMode(value)}
                      className="sr-only"
                    />
                    <ModeIcon
                      size={20}
                      weight={active ? 'fill' : 'regular'}
                      aria-hidden
                      className={active ? 'text-[var(--color-accent)]' : TEXT.muted}
                    />
                    <span className="min-w-0">
                      <span className={`block truncate text-sm ${TEXT.primary}`}>{label}</span>
                      <span className={`block truncate text-xs ${TEXT.muted}`}>{hint}</span>
                    </span>
                  </label>
                )
              })}
            </div>
          </fieldset>

          <div className="mt-5 flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <label htmlFor="email" className={`text-xs font-medium ${TEXT.secondary}`}>
                Email address
              </label>
              <input
                id="email"
                data-testid="email"
                type="email"
                autoComplete="email"
                placeholder="you@example.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                className={INPUT}
              />
            </div>

            <div className="flex flex-col gap-2">
              <label htmlFor="display-name" className={`text-xs font-medium ${TEXT.secondary}`}>
                Display name
              </label>
              <input
                id="display-name"
                type="text"
                autoComplete="name"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                className={INPUT}
              />
            </div>

            {mode === 'password' ? (
              <>
                {/* Above the host and port because it decides the port. Asked
                    for rather than guessed from the number typed: a server on
                    a non-standard port would be guessed wrong, and guessing
                    wrong means either a connection that cannot be made or one
                    made more weakly than intended. */}
                <div className="flex flex-col gap-2">
                  <span className={`text-xs font-medium ${TEXT.secondary}`}>Encryption</span>
                  <div role="radiogroup" aria-label="Encryption" className="flex gap-1.5">
                    {SECURITIES.map(({ value, label, hint }) => {
                      const active = security === value
                      return (
                        <label
                          key={value}
                          className={[
                            RADIUS,
                            'flex flex-1 cursor-pointer flex-col gap-0.5 border px-3 py-2 transition-colors',
                            active
                              ? 'border-[var(--color-accent)] bg-[var(--color-surface-selected)] dark:bg-[var(--color-surface-selected-dark)]'
                              : `${SURFACE.divider} hover:bg-neutral-100 dark:hover:bg-neutral-900`,
                          ].join(' ')}
                        >
                          <input
                            type="radio"
                            name="security"
                            value={value}
                            checked={active}
                            data-testid={`security-${value}`}
                            onChange={() => {
                              setSecurity(value)
                              // The port follows, unless the user has already
                              // moved it off a standard one — in which case
                              // they know something this code does not.
                              setImapPort((was) =>
                                was === 993 || was === 143 ? DEFAULT_PORTS[value] : was,
                              )
                            }}
                            className="sr-only"
                          />
                          <span className={`text-sm ${active ? TEXT.primary : TEXT.secondary}`}>
                            {label}
                          </span>
                          <span className={`text-xs ${TEXT.muted}`}>{hint}</span>
                        </label>
                      )
                    })}
                  </div>
                </div>

                <div className="grid grid-cols-[1fr_6rem] gap-3">
                  <div className="flex flex-col gap-2">
                    <label htmlFor="imap-host" className={`text-xs font-medium ${TEXT.secondary}`}>
                      IMAP host
                    </label>
                    <input
                      id="imap-host"
                      data-testid="imap-host"
                      type="text"
                      placeholder="imap.example.com"
                      value={imapHost}
                      onChange={(e) => setImapHost(e.target.value)}
                      className={INPUT}
                    />
                    <p className={`text-xs ${TEXT.muted}`}>
                      Leave blank for well-known providers. For on-premises
                      Exchange this is the internal server name your IT gave you.
                    </p>
                  </div>
                  <div className="flex flex-col gap-2">
                    <label htmlFor="imap-port" className={`text-xs font-medium ${TEXT.secondary}`}>
                      Port
                    </label>
                    <input
                      id="imap-port"
                      data-testid="imap-port"
                      type="number"
                      value={imapPort}
                      onChange={(e) => setImapPort(Number(e.target.value))}
                      className={`${INPUT} tabular font-mono`}
                    />
                  </div>
                </div>

                <div className="flex flex-col gap-2">
                  <label htmlFor="password" className={`text-xs font-medium ${TEXT.secondary}`}>
                    Password
                  </label>
                  <input
                    id="password"
                    data-testid="password"
                    type="password"
                    autoComplete="off"
                    placeholder="Password or app password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className={INPUT}
                  />
                </div>
              </>
            ) : (
              <p className={`text-xs leading-relaxed ${TEXT.muted}`}>
                Your browser opens so you can sign in. Nexus Mail never sees your password.
                {mode === 'google' &&
                  ' Gmail also needs an OAuth client ID of your own; see docs/oauth-setup.md.'}
              </p>
            )}

            {error && (
              <div
                role="alert"
                className={`flex gap-2 border p-3 ${RADIUS} border-[var(--color-danger)]/30`}
              >
                <Warning
                  size={ICON.size}
                  weight="fill"
                  aria-hidden
                  className="mt-0.5 shrink-0 text-[var(--color-danger)] dark:text-[var(--color-danger-dark)]"
                />
                <p className="text-xs leading-relaxed text-[var(--color-danger)] dark:text-[var(--color-danger-dark)]">
                  {error}
                </p>
              </div>
            )}

            <div className="flex items-center gap-2">
              <button
                type="button"
                data-testid="submit-account"
                onClick={submit}
                disabled={busy || email === ''}
                className={`${BUTTON_PRIMARY} inline-flex items-center gap-2 whitespace-nowrap`}
              >
                {busy && (
                  <CircleNotch size={ICON.size} weight="bold" aria-hidden className="animate-spin" />
                )}
                {busy ? 'Connecting' : 'Add account'}
              </button>

              {onCancel && (
                <button
                  type="button"
                  onClick={onCancel}
                  disabled={busy}
                  className={`${BUTTON_SECONDARY} inline-flex items-center gap-1.5 whitespace-nowrap`}
                >
                  <ArrowLeft size={ICON.size} weight={ICON.weight} aria-hidden />
                  Back
                </button>
              )}
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
