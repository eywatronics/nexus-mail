import { useEffect, useState } from 'react'
import { useMailStore } from '../store/useMailStore'

export type ThemeChoice = 'light' | 'dark' | 'system'
export type Theme = 'light' | 'dark'

export const THEME_CHOICES: readonly ThemeChoice[] = ['system', 'light', 'dark']

const DARK_QUERY = '(prefers-color-scheme: dark)'

function machinePrefersDark(): boolean {
  try {
    return window.matchMedia(DARK_QUERY).matches
  } catch {
    // Some test environments have no matchMedia. Light is the safer guess: a
    // light page on a dark machine is merely bright, where the reverse can be
    // unreadable if the palette only half applies.
    return false
  }
}

/**
 * The theme actually in force, following the machine while the choice is
 * "system".
 *
 * Shared rather than held inside the theme control, because the reading pane
 * needs it too. The pane is a sandboxed frame that cannot see the class on the
 * host page, so the theme has to travel to the backend in the body URL — and a
 * second, separately maintained answer to "are we dark right now" would drift
 * from the first the day somebody changed one of them.
 */
export function useResolvedTheme(): Theme {
  const choice = useMailStore((s) => s.themeChoice)
  const [dark, setDark] = useState(machinePrefersDark)

  useEffect(() => {
    if (choice !== 'system') return

    // Follow the machine while set to system, so the app changes with it
    // rather than only at startup — which matters for anyone whose machine
    // switches at sunset.
    let mq: MediaQueryList
    try {
      mq = window.matchMedia(DARK_QUERY)
    } catch {
      return
    }
    const onChange = () => setDark(mq.matches)
    onChange()
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [choice])

  if (choice === 'system') return dark ? 'dark' : 'light'
  return choice
}

/**
 * Puts the theme on the document, and returns it.
 *
 * Called once, by the shell. It used to live inside the theme control, which
 * worked only for as long as that control was on screen — the moment the
 * control moved into a settings screen the app would have rendered unstyled
 * everywhere else. Applying a document-wide effect from a component that can
 * unmount was the bug waiting to happen; the shell cannot unmount.
 */
export function useApplyTheme(): Theme {
  const theme = useResolvedTheme()

  useEffect(() => {
    document.documentElement.classList.toggle('dark', theme === 'dark')
  }, [theme])

  return theme
}
