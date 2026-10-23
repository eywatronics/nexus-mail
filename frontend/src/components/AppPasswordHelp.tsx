import { ArrowRight, ArrowSquareOut, Key, Warning } from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { openExternal } from '../lib/external'
import { ICON, LINK, RADIUS, SURFACE, TEXT } from '../lib/ui'

export type PasswordProvider = 'google' | 'microsoft' | 'apple' | 'yahoo' | 'other'

/**
 * Only domains the backend already has a preset server for.
 *
 * Advice about a provider whose host this build cannot fill in would be half
 * an answer: the reader would still be left with an empty server field and no
 * way to know what belongs in it.
 */
const PROVIDERS: Array<{
  name: Exclude<PasswordProvider, 'other'>
  domains: string[]
}> = [
  { name: 'google', domains: ['gmail.com', 'googlemail.com'] },
  {
    name: 'microsoft',
    domains: ['outlook.com', 'hotmail.com', 'live.com', 'msn.com', 'office365.com'],
  },
  { name: 'apple', domains: ['icloud.com', 'me.com'] },
  { name: 'yahoo', domains: ['yahoo.com'] },
]

const GOOGLE_SECURITY = 'https://myaccount.google.com/security'
const GOOGLE_APP_PASSWORDS = 'https://myaccount.google.com/apppasswords'
const APPLE_ACCOUNT = 'https://account.apple.com/'
const YAHOO_SECURITY = 'https://login.yahoo.com/account/security'

export function passwordProviderFor(email: string): PasswordProvider {
  const at = email.lastIndexOf('@')
  if (at < 1 || at === email.length - 1) return 'other'

  const domain = email.slice(at + 1).trim().toLowerCase()
  for (const provider of PROVIDERS) {
    if (provider.domains.includes(domain)) return provider.name
  }
  return 'other'
}

const BODY = `mt-1 text-xs leading-relaxed ${TEXT.secondary}`
const STEPS = `mt-2 flex list-decimal flex-col gap-1 pl-4 text-xs leading-relaxed ${TEXT.secondary}`
const FINE_PRINT = `mt-2 text-xs leading-relaxed ${TEXT.muted}`

/**
 * A provider page, opened in the reader's own browser.
 *
 * A real anchor cannot be used: the WebView would navigate the application
 * away from the form. The title carries the address for anyone who would
 * rather check where a link goes before clicking it.
 */
function ExternalPage({
  href,
  testId,
  children,
}: {
  href: string
  testId: string
  children: ReactNode
}) {
  return (
    <button
      type="button"
      data-testid={testId}
      title={href}
      onClick={() => {
        // A browser that will not open is not worth an error inside the form,
        // but an unhandled rejection would reach the console as noise.
        void openExternal(href).catch(() => {})
      }}
      className={LINK}
    >
      {children}
      <ArrowSquareOut size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
    </button>
  )
}

function SwitchMode({
  testId,
  onClick,
  children,
}: {
  testId: string
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button type="button" data-testid={testId} onClick={onClick} className={`${LINK} mt-2`}>
      {children}
      <ArrowRight size={ICON.size} weight={ICON.weight} aria-hidden className="shrink-0" />
    </button>
  )
}

function Panel({
  testId,
  labelId,
  title,
  icon,
  warning = false,
  children,
}: {
  testId: string
  labelId: string
  title: string
  icon: ReactNode
  warning?: boolean
  children: ReactNode
}) {
  return (
    <section
      data-testid={testId}
      aria-labelledby={labelId}
      className={`border p-3 ${RADIUS} ${
        warning ? 'border-[var(--color-warn)]/40' : SURFACE.divider
      }`}
    >
      <div className="flex gap-2.5">
        {icon}
        <div className="min-w-0">
          <h3 id={labelId} className={`text-xs font-medium ${TEXT.primary}`}>
            {title}
          </h3>
          {children}
        </div>
      </div>
    </section>
  )
}

/**
 * What the form needs to say about the address typed into it.
 *
 * This exists because an app password is the only way past the OAuth client-ID
 * wall for a Gmail user, and because Microsoft closed that door: Outlook.com
 * stopped accepting app passwords for IMAP in 2024, so the honest answer there
 * is to point at the sign-in that does work rather than at a password form
 * that cannot.
 */
export function AppPasswordHelp({
  email,
  assumeGoogle = false,
  onUseMicrosoftSignIn,
}: {
  email: string
  /**
   * True when the reader arrived here from the Gmail sign-in. A Workspace
   * account on a custom domain cannot be spotted by its address, and the
   * instructions are the same ones Gmail needs.
   */
  assumeGoogle?: boolean
  onUseMicrosoftSignIn: () => void
}) {
  const detected = passwordProviderFor(email)
  const provider = assumeGoogle && detected === 'other' ? 'google' : detected

  if (provider === 'google') {
    return (
      <Panel
        testId="app-password-google"
        labelId="gmail-app-password"
        title="Gmail needs an app password"
        icon={
          <Key
            size={20}
            weight="light"
            aria-hidden
            className="mt-0.5 shrink-0 text-[var(--color-accent)]"
          />
        }
      >
        <p className={BODY}>
          Google does not accept your normal password in mail apps. An app password is
          the way in.
        </p>
        <ol className={STEPS}>
          <li>
            Turn on{' '}
            <ExternalPage href={GOOGLE_SECURITY} testId="open-google-2fa">
              2-Step Verification
            </ExternalPage>
            .
          </li>
          <li>
            Create an{' '}
            <ExternalPage href={GOOGLE_APP_PASSWORDS} testId="open-google-app-passwords">
              app password
            </ExternalPage>
            .
          </li>
          <li>Paste the 16 characters above. Spaces do not matter.</li>
        </ol>
        <p className={FINE_PRINT}>
          Google Workspace: an administrator may have turned app passwords off.
        </p>
      </Panel>
    )
  }

  if (provider === 'microsoft') {
    return (
      <Panel
        testId="app-password-microsoft"
        labelId="microsoft-app-password"
        title="Microsoft no longer accepts passwords"
        warning
        icon={
          <Warning
            size={20}
            weight="light"
            aria-hidden
            className="mt-0.5 shrink-0 text-[var(--color-warn)] dark:text-[var(--color-warn-dark)]"
          />
        }
      >
        <p className={BODY}>
          Outlook.com stopped accepting app passwords for IMAP in 2024, and Microsoft
          365 requires its own sign-in. A password cannot connect this account.
        </p>
        <SwitchMode testId="use-microsoft-signin" onClick={onUseMicrosoftSignIn}>
          Use Microsoft sign-in
        </SwitchMode>
      </Panel>
    )
  }

  if (provider === 'apple') {
    return (
      <Panel
        testId="app-password-apple"
        labelId="icloud-app-password"
        title="iCloud needs an app-specific password"
        icon={
          <Key
            size={20}
            weight="light"
            aria-hidden
            className="mt-0.5 shrink-0 text-[var(--color-accent)]"
          />
        }
      >
        <p className={BODY}>
          Your Apple ID password will not work in a mail app. Create an app-specific
          password and paste it above.
        </p>
        <p className="mt-2">
          <ExternalPage href={APPLE_ACCOUNT} testId="open-apple-account">
            Create an app-specific password
          </ExternalPage>
        </p>
      </Panel>
    )
  }

  if (provider === 'yahoo') {
    return (
      <Panel
        testId="app-password-yahoo"
        labelId="yahoo-app-password"
        title="Yahoo needs an app password"
        icon={
          <Key
            size={20}
            weight="light"
            aria-hidden
            className="mt-0.5 shrink-0 text-[var(--color-accent)]"
          />
        }
      >
        <p className={BODY}>
          Yahoo asks for an app password when two-step verification is on. Create one
          and paste it above.
        </p>
        <p className="mt-2">
          <ExternalPage href={YAHOO_SECURITY} testId="open-yahoo-security">
            Create a Yahoo app password
          </ExternalPage>
        </p>
      </Panel>
    )
  }

  return null
}
