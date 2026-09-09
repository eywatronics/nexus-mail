import { useEffect, useState } from 'react'

type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'nexus-mail-theme'

function apply(theme: Theme) {
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  const dark = theme === 'dark' || (theme === 'system' && prefersDark)
  document.documentElement.classList.toggle('dark', dark)
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem(STORAGE_KEY) as Theme | null) ?? 'system',
  )

  useEffect(() => {
    apply(theme)
    localStorage.setItem(STORAGE_KEY, theme)

    if (theme !== 'system') return

    // Follow the OS while set to system, so the app changes with it rather
    // than only at startup — which matters for anyone whose machine switches
    // at sunset.
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => apply('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [theme])

  return (
    <select
      value={theme}
      onChange={(e) => setTheme(e.target.value as Theme)}
      aria-label="Theme"
      className="rounded border border-neutral-300 bg-transparent px-2 py-1 text-xs dark:border-neutral-700"
    >
      <option value="system">System</option>
      <option value="light">Light</option>
      <option value="dark">Dark</option>
    </select>
  )
}
