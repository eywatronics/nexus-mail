import { ArrowLeft, Warning } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import { settings as readSettings, updateSettings, type AppSettings } from '../lib/api'
import { BODY_VIEWS, useBodyView } from '../lib/bodyView'
import { MARK_READ_OPTIONS, useMarkReadWhen } from '../lib/markRead'
import { useMailStore } from '../store/useMailStore'
import { BUTTON_SECONDARY, ICON, RADIUS, SURFACE, TEXT } from '../lib/ui'
import { Choice, NumberSetting, Setting, SettingsSection, TextSetting, Toggle } from './SettingsControls'

const THEME_OPTIONS = [
  { value: 'system' as const, label: 'System' },
  { value: 'light' as const, label: 'Light' },
  { value: 'dark' as const, label: 'Dark' },
]

/**
 * Everything the app can be told to do differently.
 *
 * Built because three separate settings had already been written with a
 * comment saying "when a preferences screen exists, this belongs there", and
 * because the OAuth client id — the first thing a new account needs — could
 * only be set by finding a JSON file in a directory the app never names.
 *
 * Nothing here has a Save button. Choices apply as they are made and text
 * fields on the way out of them, which is what every settings surface people
 * use has converged on; a Save button adds a state to get wrong ("did that
 * take?") in exchange for nothing.
 */
export function Settings({ onClose }: { onClose: () => void }) {
  const [stored, setStored] = useState<AppSettings | null>(null)
  const [error, setError] = useState('')

  const themeChoice = useMailStore((s) => s.themeChoice)
  const setThemeChoice = useMailStore((s) => s.setThemeChoice)
  const threaded = useMailStore((s) => s.threaded)
  const setThreaded = useMailStore((s) => s.setThreaded)
  const [bodyView, setBodyView] = useBodyView()
  const [markReadWhen, setMarkReadWhen] = useMarkReadWhen()

  useEffect(() => {
    readSettings()
      .then(setStored)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
  }, [])

  // The saved copy is replaced with what was sent rather than re-read. A
  // re-read would be a second round trip to learn what we already know, and it
  // would blank the fields for a frame while it happened.
  const save = (next: AppSettings) => {
    const previous = stored
    setStored(next)
    setError('')
    updateSettings(next).catch((err: unknown) => {
      setStored(previous)
      setError(err instanceof Error ? err.message : String(err))
    })
  }

  return (
    <div className={`h-full overflow-y-auto ${SURFACE.page}`}>
      <div className="mx-auto max-w-3xl px-8 py-12">
        <div className="flex items-baseline justify-between gap-4">
          <h1 className={`text-2xl font-semibold tracking-tight ${TEXT.primary}`}>Settings</h1>
          <button
            type="button"
            data-testid="settings-close"
            onClick={onClose}
            className={`${BUTTON_SECONDARY} inline-flex items-center gap-1.5 py-1.5`}
          >
            <ArrowLeft size={ICON.size} weight={ICON.weight} aria-hidden />
            Back to mail
          </button>
        </div>

        {error && (
          <div
            role="alert"
            data-testid="settings-error"
            className={`mt-6 flex gap-2 border p-3 ${RADIUS} border-[var(--color-danger)]/30`}
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

        <div className="mt-8">
          <SettingsSection title="Appearance">
            <Setting label="Theme" hint="Following the system changes with it, including at sunset.">
              <Choice name="theme" value={themeChoice} options={THEME_OPTIONS} onChange={setThemeChoice} />
            </Setting>

            <Setting
              label="Message body"
              hint="Simple drops the sender's colours and layout but keeps the structure. Plain text drops the markup entirely."
            >
              <Choice
                name="body-view"
                value={bodyView}
                options={BODY_VIEWS.map((v) => ({ value: v.value, label: v.label }))}
                onChange={setBodyView}
              />
            </Setting>
          </SettingsSection>

          <SettingsSection title="Reading">
            <Setting
              label="Mark messages read"
              hint={MARK_READ_OPTIONS.find((o) => o.value === markReadWhen)?.hint}
            >
              <Choice
                name="mark-read"
                value={markReadWhen}
                options={MARK_READ_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
                onChange={setMarkReadWhen}
              />
            </Setting>

            <Setting
              label="Group by conversation"
              hint="Replies are folded under the message they answer, newest conversation first."
            >
              <Toggle name="threaded" value={threaded} onChange={setThreaded} />
            </Setting>
          </SettingsSection>

          {stored && (
            <>
              <SettingsSection title="Background">
                <Setting
                  label="Start when I sign in"
                  hint={
                    stored.startAtLoginAvailable
                      ? 'Nexus Mail opens in the tray and starts syncing, without a window. New mail arrives without you having launched anything.'
                      : 'This build cannot register itself to start at login — on Linux that usually means it is running from an AppImage with no autostart directory.'
                  }
                >
                  <Toggle
                    name="start-at-login"
                    value={stored.startAtLogin}
                    disabled={!stored.startAtLoginAvailable}
                    onChange={(next) => save({ ...stored, startAtLogin: next })}
                  />
                </Setting>
              </SettingsSection>

              <SettingsSection title="Privacy">
                <Setting
                  label="Show who and what in notifications"
                  hint="Windows shows notifications on the lock screen unless told otherwise. With this off, a new-mail notification says only how many arrived."
                >
                  <Toggle
                    name="notification-preview"
                    value={stored.notificationPreview}
                    onChange={(next) => save({ ...stored, notificationPreview: next })}
                  />
                </Setting>
              </SettingsSection>

              <SettingsSection title="Deleting">
                <Setting
                  label="Undo window"
                  hint="How long a delete or a move is held before it goes to the server. Zero sends it at once and offers no undo."
                  htmlFor="undoWindowSeconds"
                >
                  <NumberSetting
                    id="undoWindowSeconds"
                    value={stored.undoWindowSeconds}
                    unit="seconds"
                    onCommit={(next) => save({ ...stored, undoWindowSeconds: next })}
                  />
                </Setting>
              </SettingsSection>

              <SettingsSection title="Storage">
                <Setting
                  label="Keep mail for"
                  hint="Older messages are removed from this machine only; they stay on the server. Starred messages are never removed. Zero keeps everything."
                  htmlFor="retentionDays"
                >
                  <NumberSetting
                    id="retentionDays"
                    value={stored.retentionDays}
                    unit="days"
                    onCommit={(next) => save({ ...stored, retentionDays: next })}
                  />
                </Setting>

                <Setting
                  label="Keep at most"
                  hint="Per folder. Zero keeps everything."
                  htmlFor="retentionMaxMessages"
                >
                  <NumberSetting
                    id="retentionMaxMessages"
                    value={stored.retentionMaxMessages}
                    unit="messages"
                    onCommit={(next) => save({ ...stored, retentionMaxMessages: next })}
                  />
                </Setting>
              </SettingsSection>

              <SettingsSection title="Sign-in with Google or Microsoft">
                <p className={`pb-2 text-xs leading-relaxed ${TEXT.secondary}`}>
                  These accounts need an OAuth client id of your own — a desktop app cannot keep a
                  secret on your machine, so registering your own is what keeps access to your
                  mailbox under your control. The setup takes a few minutes and is described in
                  docs/oauth-setup.md. An on-premises server needs none of this.{' '}
                  <span className={TEXT.muted}>Changes here take effect after a restart.</span>
                </p>

                <Setting label="Google client id" htmlFor="googleClientId">
                  <TextSetting
                    id="googleClientId"
                    value={stored.googleClientId}
                    placeholder="…apps.googleusercontent.com"
                    onCommit={(next) => save({ ...stored, googleClientId: next })}
                  />
                </Setting>

                <Setting label="Microsoft client id" htmlFor="microsoftClientId">
                  <TextSetting
                    id="microsoftClientId"
                    value={stored.microsoftClientId}
                    placeholder="Application (client) ID"
                    onCommit={(next) => save({ ...stored, microsoftClientId: next })}
                  />
                </Setting>

                <Setting
                  label="Sign-in port"
                  hint="Zero picks a free one, which is right almost always. Set it only if security software blocks that, and register the same port with the provider."
                  htmlFor="oauthRedirectPort"
                >
                  <NumberSetting
                    id="oauthRedirectPort"
                    value={stored.oauthRedirectPort}
                    unit="port"
                    onCommit={(next) => save({ ...stored, oauthRedirectPort: next })}
                  />
                </Setting>
              </SettingsSection>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
