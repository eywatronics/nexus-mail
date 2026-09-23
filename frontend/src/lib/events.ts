/**
 * Event names must match internal/app/events.go exactly.
 *
 * There are three, so duplicating them costs less than generating them. If the
 * list grows, generate it from the Go constants instead.
 */
export const EVENTS = {
  syncStarted: 'sync:started',
  syncFinished: 'sync:finished',
  syncFailed: 'sync:failed',
} as const

export type ErrorClass = 'transient' | 'auth' | 'protocol' | 'permanent'

export interface SyncEventPayload {
  accountId: number
  email: string
  error?: string
  class?: ErrorClass
}
