import { Desktop, Moon, Sun } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { useEffect, useState } from 'react'
import { ICON, RADIUS, TEXT } from '../lib/ui'

type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'nexus-mail-theme'

const OPTIONS: Array<[Theme, string, Icon]> = [
  ['system', 'Match system', Desktop],
  ['light', 'Light', Sun],
  ['dark', 'Dark', Moon],
]

function apply(theme: Theme) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const dark = theme === 'dark' || (theme === 'system' && prefersDark)
  document.documentElement.classList.toggle('dark', dark)
}

/**
 * A three-way segmented control rather than a cycling button: with a cycle the
 * user has to click and watch to discover what the next state is, and "system"
 * is invisible as a concept. Three labelled options show the whole choice.
 */
export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem(STORAGE_KEY) as Theme | null) ?? 'system',
  )

  useEffect(() => {
    apply(theme)
    localStorage.setItem(STORAGE_KEY, theme)

    if (theme !== 'system') return

    // Follow the OS while set to system, so the app changes with it rather
    // than only at startup, which matters for anyone whose machine switches at
    // sunset.
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => apply('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [theme])

  return (
    <div
      role="radiogroup"
      aria-label="Theme"
      className={`flex items-center gap-0.5 border border-neutral-200 p-0.5 dark:border-neutral-800 ${RADIUS}`}
    >
      {OPTIONS.map(([value, label, OptionIcon]) => {
        const active = theme === value
        return (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={active}
            aria-label={label}
            title={label}
            onClick={() => setTheme(value)}
            className={[
              RADIUS,
              'p-1 transition-colors',
              active
                ? `bg-neutral-200 ${TEXT.primary} dark:bg-neutral-800`
                : `${TEXT.muted} hover:bg-neutral-100 dark:hover:bg-neutral-900`,
            ].join(' ')}
          >
            <OptionIcon size={ICON.size} weight={active ? 'fill' : ICON.weight} />
          </button>
        )
      })}
    </div>
  )
}
