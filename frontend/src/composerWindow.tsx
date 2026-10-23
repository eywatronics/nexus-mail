import { Window } from '@wailsio/runtime'
import { useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Composer } from './components/Composer'
import { takeComposerWindow, type DraftRecord } from './lib/api'
import { useApplyTheme } from './lib/theme'
import { BUTTON_PRIMARY, TEXT } from './lib/ui'
import './index.css'

/**
 * The shell for a composer that lives in its own operating-system window.
 *
 * It fetches the draft the main window handed to the backend, then gets out of
 * the way. The id is one-shot: a reload shows an expired notice rather than
 * opening a second composer over the same draft.
 */
function ComposerWindow() {
  const [draft, setDraft] = useState<DraftRecord | null | undefined>(undefined)
  useApplyTheme()

  const id = new URLSearchParams(window.location.search).get('id') ?? ''
  const close = () => {
    void Window.Close()
  }

  useEffect(() => {
    if (id === '') {
      setDraft(null)
      return
    }
    takeComposerWindow(id)
      .then(setDraft)
      .catch(() => setDraft(null))
  }, [id])

  if (draft === undefined) {
    return <div className="h-full" aria-busy="true" />
  }

  if (draft === null) {
    return (
      <main className="flex h-full flex-col items-center justify-center gap-4 px-6 text-center">
        <p className={`text-sm ${TEXT.muted}`}>
          This composer window has expired.
        </p>
        <button type="button" onClick={close} className={`${BUTTON_PRIMARY} px-3 py-1.5`}>
          Close
        </button>
      </main>
    )
  }

  return (
    <Composer
      accountId={draft.accountId}
      draft={draft}
      onClose={close}
    />
  )
}

createRoot(document.getElementById('root')!).render(<ComposerWindow />)
