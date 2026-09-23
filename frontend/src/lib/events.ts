/**
 * Event names must match internal/app/events.go exactly.
 *
 * There are four, so duplicating them costs less than generating them. If the
 * list grows, generate it from the Go constants instead.
 */
export const EVENTS = {
  syncStarted: 'sync:started',
  syncFinished: 'sync:finished',
  syncFailed: 'sync:failed',
} as const

/**
 * Deliberately not in EVENTS: the shell subscribes to every name in there and
 * routes it to the sync reducer. This one belongs to the reading pane, which
 * subscribes to it itself.
 */
export const FIND_RESULTS_EVENT = 'find:results'

/**
 * What a find in the reading pane turned up.
 *
 * It arrives as an event rather than a return value because the counting
 * happens while the body is being served: the frame requests a document with a
 * query on it, and the number of matches falls out of building that document.
 * Asking separately would render the message twice and could disagree with
 * what is on screen.
 */
export interface FindResultsPayload {
  messageId: number
  query: string
  count: number
  /** The match the document scrolled to, after the asked-for index was wrapped. */
  current: number
}

export type ErrorClass = 'transient' | 'auth' | 'protocol' | 'permanent'

export interface SyncEventPayload {
  accountId: number
  email: string
  error?: string
  class?: ErrorClass
}
