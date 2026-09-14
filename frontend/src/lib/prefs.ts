/**
 * Small, per-person display settings that belong to the window.
 *
 * Not the backend's config.json: these are properties of how somebody reads,
 * not of an account, and they have to be readable while the first frame is
 * drawn rather than after a round trip. The same storage the theme control
 * uses, for the same reason.
 *
 * Every access is guarded. A browser with storage disabled is not a reason to
 * fail to draw a message; it just means the choice lasts one session.
 */
export function readPref<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const saved = localStorage.getItem(key)
    if (saved !== null && (allowed as readonly string[]).includes(saved)) {
      return saved as T
    }
  } catch {
    // Storage is unavailable; the fallback is the answer.
  }
  return fallback
}

export function writePref(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Losing the preference is not worth an error.
  }
}
