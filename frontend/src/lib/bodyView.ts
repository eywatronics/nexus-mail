import { useEffect, useState } from 'react'
import { readPref, writePref } from './prefs'

/** How much of the sender's presentation the reading pane shows. */
export type BodyView = 'rich' | 'simple' | 'text'

export const BODY_VIEWS: Array<{ value: BodyView; label: string }> = [
  { value: 'rich', label: 'Original HTML' },
  { value: 'simple', label: 'Simple HTML' },
  { value: 'text', label: 'Plain text' },
]

const STORAGE_KEY = 'nexus-mail-body-view'
const VALUES = BODY_VIEWS.map((v) => v.value)

/**
 * The reading pane's view mode, remembered across restarts.
 *
 * Sticky rather than per message on purpose. Somebody who prefers plain text
 * prefers it for their mail, not for one message, and re-choosing it on every
 * message would make the setting useless.
 *
 * When a preferences screen exists this is one of the settings it should show,
 * rather than a second place to set it.
 */
export function useBodyView(): [BodyView, (next: BodyView) => void] {
  const [view, setView] = useState<BodyView>(() => readPref(STORAGE_KEY, VALUES, 'rich'))

  useEffect(() => {
    writePref(STORAGE_KEY, view)
  }, [view])

  return [view, setView]
}
