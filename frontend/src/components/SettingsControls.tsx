import type { ReactNode } from 'react'
import { ICON, INPUT, RADIUS, SURFACE, TEXT } from '../lib/ui'

/**
 * One labelled setting.
 *
 * Label and explanation on the left, control on the right, separated by a
 * hairline rather than boxed into a card. At this density a card per setting
 * would spend more space on chrome than on the settings, and the rule already
 * does the grouping.
 */
export function Setting({
  label,
  hint,
  htmlFor,
  children,
}: {
  label: string
  hint?: string
  htmlFor?: string
  children: ReactNode
}) {
  return (
    <div
      className={`flex flex-wrap items-start justify-between gap-x-6 gap-y-2 border-b py-4 ${SURFACE.divider}`}
    >
      <div className="min-w-0 max-w-[42ch]">
        <label htmlFor={htmlFor} className={`block text-sm font-medium ${TEXT.primary}`}>
          {label}
        </label>
        {hint && <p className={`mt-1 text-xs leading-relaxed ${TEXT.secondary}`}>{hint}</p>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

/** A group of settings under a heading. */
export function SettingsSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="mt-10 first:mt-0">
      <h2 className={`text-xs font-semibold uppercase tracking-wider ${TEXT.muted}`}>{title}</h2>
      <div className="mt-2">{children}</div>
    </section>
  )
}

/**
 * A segmented choice, for settings with a handful of named answers.
 *
 * The same shape as the theme control, because it is the same kind of
 * question: with a cycling button the reader has to click and watch to
 * discover what the next state is, and a dropdown hides the answers until
 * asked. Three visible options show the whole choice.
 */
export function Choice<T extends string>({
  value,
  options,
  onChange,
  name,
}: {
  value: T
  options: ReadonlyArray<{ value: T; label: string }>
  onChange: (next: T) => void
  name: string
}) {
  return (
    <div
      role="radiogroup"
      aria-label={name}
      className={`flex items-center gap-0.5 border p-0.5 ${RADIUS} ${SURFACE.divider}`}
    >
      {options.map((option) => {
        const active = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={active}
            data-testid={`${name}-${option.value}`}
            onClick={() => onChange(option.value)}
            className={[
              RADIUS,
              'whitespace-nowrap px-2.5 py-1 text-xs transition-colors',
              active
                ? `bg-neutral-200 ${TEXT.primary} dark:bg-neutral-800`
                : `${TEXT.secondary} hover:bg-neutral-100 dark:hover:bg-neutral-900`,
            ].join(' ')}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}

/**
 * A yes/no setting.
 *
 * Drawn as two words rather than as a sliding switch. A switch has to be read
 * twice — once for its position and once for its label — and at this size the
 * position is the part that is hard to read.
 */
export function Toggle({
  value,
  onChange,
  name,
}: {
  value: boolean
  onChange: (next: boolean) => void
  name: string
}) {
  return (
    <Choice
      name={name}
      value={value ? 'on' : 'off'}
      options={[
        { value: 'on', label: 'On' },
        { value: 'off', label: 'Off' },
      ]}
      onChange={(next) => onChange(next === 'on')}
    />
  )
}

/**
 * A number with a unit after it.
 *
 * Saved on blur rather than on every keystroke: a partially typed number is
 * not a setting, and writing "1" on the way to "10" would briefly mean
 * something quite different.
 */
export function NumberSetting({
  id,
  value,
  unit,
  onCommit,
}: {
  id: string
  value: number
  unit: string
  onCommit: (next: number) => void
}) {
  return (
    <span className="flex items-center gap-2">
      <input
        id={id}
        data-testid={id}
        type="number"
        min={0}
        defaultValue={value}
        key={value}
        onBlur={(event) => {
          const next = Number(event.target.value)
          if (Number.isFinite(next) && next !== value) onCommit(next)
        }}
        className={`${INPUT} tabular w-28 py-1.5 font-mono text-sm`}
      />
      <span className={`text-xs ${TEXT.muted}`}>{unit}</span>
    </span>
  )
}

/** A free-text setting, saved on blur for the same reason. */
export function TextSetting({
  id,
  value,
  placeholder,
  onCommit,
}: {
  id: string
  value: string
  placeholder?: string
  onCommit: (next: string) => void
}) {
  return (
    <input
      id={id}
      data-testid={id}
      type="text"
      spellCheck={false}
      autoComplete="off"
      placeholder={placeholder}
      defaultValue={value}
      key={value}
      onBlur={(event) => {
        const next = event.target.value.trim()
        if (next !== value) onCommit(next)
      }}
      className={`${INPUT} w-72 py-1.5 font-mono text-xs`}
    />
  )
}

export const SETTINGS_ICON = ICON
