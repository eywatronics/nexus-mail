import { useEffect, useState } from 'react'

/** How much of the sender's presentation the reading pane shows. */
export type BodyView = 'rich' | 'simple' | 'text'

export const BODY_VIEWS: Array<{ value: BodyView; label: string }> = [
  { value: 'rich', label: 'Original HTML' },
  { value: 'simple', label: 'Simple HTML' },
  { value: 'text', label: 'Plain text' },
]

const STORAGE_KEY = 'nexus-mail-body-view'

function stored(): BodyView {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved === 'simple' || saved === 'text' || saved === 'rich') return saved
  } catch {
    // A browser with storage disabled is not a reason to fail to draw a
    // message; it just means the choice lasts one session.
  }
  return 'rich'
}

/**
 * The reading pane's view mode, remembered across restarts.
 *
 * Sticky rather than per message on purpose. Somebody who prefers plain text
 * prefers it for their mail, not for one message, and re-choosing it on every
 * message would make the setting useless. This is the same reasoning — and the
 * same storage — as the theme control.
 *
 * It lives in the window rather than in the backend's config.json because it
 * is a property of how this person reads, not of the account. A preferences
 * screen does not exist yet; when it does, this is one of the settings it
 * should show rather than a second place to set it.
 */
export function useBodyView(): [BodyView, (next: BodyView) => void] {
  const [view, setView] = useState<BodyView>(stored)

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, view)
    } catch {
      // As above: losing the preference is not worth an error.
    }
  }, [view])

  return [view, setView]
}
