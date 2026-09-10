/**
 * Shared class strings for the interface.
 *
 * These exist so the accent color and the corner radius live in one place.
 * Scattering `bg-blue-600` and `rounded-lg` across components is how a UI ends
 * up with four accents and three radii that nobody chose — every one of them
 * looked reasonable at its own call site.
 */

/** Every interactive element and container shares this radius. */
export const RADIUS = 'rounded-[var(--radius-ui)]'

/** Primary action. One per screen. */
export const BUTTON_PRIMARY = [
  RADIUS,
  'bg-[var(--color-accent)] px-4 py-2 text-sm font-medium text-white',
  'transition-colors hover:bg-[var(--color-accent-hover)]',
  // Physical press feedback, per the tactile rule. Cheap and instant.
  'active:translate-y-px',
  'disabled:cursor-not-allowed disabled:opacity-50 disabled:active:translate-y-0',
].join(' ')

/** Secondary action. Bordered, never a second colour. */
export const BUTTON_SECONDARY = [
  RADIUS,
  'border border-neutral-300 px-4 py-2 text-sm text-neutral-800',
  'transition-colors hover:bg-neutral-100 active:translate-y-px',
  'dark:border-neutral-700 dark:text-neutral-200 dark:hover:bg-neutral-800',
  'disabled:cursor-not-allowed disabled:opacity-50 disabled:active:translate-y-0',
].join(' ')

/** Quiet action for toolbars. No border until hovered. */
export const BUTTON_GHOST = [
  RADIUS,
  'px-2 py-1 text-xs text-neutral-700 transition-colors',
  'hover:bg-neutral-200 dark:text-neutral-300 dark:hover:bg-neutral-800',
].join(' ')

export const INPUT = [
  RADIUS,
  'border border-neutral-300 bg-white px-3 py-2 text-sm text-neutral-900',
  // Placeholder at 500 rather than 400: on white, neutral-400 is about 2.8:1
  // and fails WCAG AA.
  'placeholder:text-neutral-500',
  'dark:border-neutral-700 dark:bg-neutral-900 dark:text-neutral-100',
  'dark:placeholder:text-neutral-400',
].join(' ')

/**
 * Text tones, contrast-checked in both modes.
 *
 * `neutral-400` on white is roughly 2.8:1 and fails AA, so the light side
 * never goes below 500. On near-black the same value is comfortably above 7:1,
 * which is why the two sides are not symmetrical.
 */
export const TEXT = {
  primary: 'text-neutral-900 dark:text-neutral-100',
  secondary: 'text-neutral-600 dark:text-neutral-300',
  muted: 'text-neutral-500 dark:text-neutral-400',
} as const

/** Panel and divider surfaces. Square edges; only content inside is rounded. */
export const SURFACE = {
  panel: 'bg-neutral-50 dark:bg-neutral-900',
  page: 'bg-white dark:bg-neutral-950',
  divider: 'border-neutral-200 dark:border-neutral-800',
} as const

/**
 * Selected row in a list: a neutral lift plus a left accent bar.
 *
 * An accent-tinted fill was the first attempt and it failed in dark mode. Text
 * tones are contrast-checked against the surface they sit on, and a tinted row
 * is a different surface: the muted snippet line dropped to roughly 2.4:1 on
 * the selected row while passing everywhere else. A neutral lift keeps every
 * row's text on nearly the same background, so the contrast math holds, and
 * the bar carries the accent without getting under the type.
 *
 * The lift is deliberately faint. The bar is what says "this one"; a heavier
 * background only moves the muted text closer to failing again, which is how
 * the first two attempts at this went.
 *
 * Unselected rows reserve the same 2px with a transparent border, otherwise
 * selecting a row would shift its text sideways.
 */
export const SELECTED =
  'border-l-2 border-[var(--color-accent)] ' +
  'bg-[var(--color-surface-selected)] dark:bg-[var(--color-surface-selected-dark)]'
export const UNSELECTED_BAR = 'border-l-2 border-transparent'

/** One icon size and weight for the whole app, per the icon policy. */
export const ICON = { size: 16, weight: 'regular' } as const
