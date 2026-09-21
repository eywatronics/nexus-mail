import { Desktop, Moon, Sun } from '@phosphor-icons/react'
import type { Icon } from '@phosphor-icons/react'
import { useEffect } from 'react'
import { useResolvedTheme, type ThemeChoice } from '../lib/theme'
import { useMailStore } from '../store/useMailStore'
import { ICON, RADIUS, TEXT } from '../lib/ui'

const OPTIONS: Array<[ThemeChoice, string, Icon]> = [
  ['system', 'Match system', Desktop],
  ['light', 'Light', Sun],
  ['dark', 'Dark', Moon],
]

/**
 * A three-way segmented control rather than a cycling button: with a cycle the
 * user has to click and watch to discover what the next state is, and "system"
 * is invisible as a concept. Three labelled options show the whole choice.
 *
 * The choice itself lives in the store, because the reading pane needs it too
 * — it is a sandboxed frame that cannot see the class set here, so the theme
 * has to reach it through the body URL.
 */
export function ThemeToggle() {
  const theme = useMailStore((s) => s.themeChoice)
  const setTheme = useMailStore((s) => s.setThemeChoice)
  const resolved = useResolvedTheme()

  useEffect(() => {
    document.documentElement.classList.toggle('dark', resolved === 'dark')
  }, [resolved])

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
