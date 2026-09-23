import { useEffect, useState } from 'react'
import { readPref, writePref } from './prefs'

/** When an opened message counts as read. */
export type MarkReadWhen = 'open' | 'delay' | 'never'

export const MARK_READ_OPTIONS: Array<{ value: MarkReadWhen; label: string; hint: string }> = [
  { value: 'open', label: 'When I open it', hint: 'As soon as the message is shown' },
  { value: 'delay', label: 'After a few seconds', hint: 'Skipping past it leaves it unread' },
  { value: 'never', label: 'Only when I say so', hint: 'Use the toolbar or the r key' },
]

/** How long "a few seconds" is. */
export const MARK_READ_DELAY_MS = 3000

const STORAGE_KEY = 'nexus-mail-mark-read'
const VALUES = MARK_READ_OPTIONS.map((o) => o.value)

/**
 * When a message the reader opened becomes read.
 *
 * Three answers because people genuinely want different ones. Somebody who
 * triages by arrow key wants the delay, so passing over a message does not
 * clear the bold; somebody who reads everything wants it marked on sight; and
 * somebody who uses unread as a to-do list wants it never touched except by
 * their own hand.
 *
 * The default stays "when I open it", which is what the app did before this
 * setting existed — a setting that quietly changed the behaviour of everyone
 * who never opened it would be a worse feature than no setting.
 */
export function useMarkReadWhen(): [MarkReadWhen, (next: MarkReadWhen) => void] {
  const [when, setWhen] = useState<MarkReadWhen>(() => readPref(STORAGE_KEY, VALUES, 'open'))

  useEffect(() => {
    writePref(STORAGE_KEY, when)
  }, [when])

  return [when, setWhen]
}
