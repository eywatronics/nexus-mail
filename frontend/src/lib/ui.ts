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
  secondary: 'text-neutral-600 dark:text-neutral-400',
  muted: 'text-neutral-500 dark:text-neutral-500',
} as const

/** Panel and divider surfaces. Square edges; only content inside is rounded. */
export const SURFACE = {
  panel: 'bg-neutral-50 dark:bg-neutral-900',
  page: 'bg-white dark:bg-neutral-950',
  divider: 'border-neutral-200 dark:border-neutral-800',
} as const

/** Selected row in a list. The only place the accent tints a background. */
export const SELECTED =
  'bg-[var(--color-accent-soft)] dark:bg-[var(--color-accent-soft-dark)]'

/** One icon size and weight for the whole app, per the icon policy. */
export const ICON = { size: 16, weight: 'regular' } as const
