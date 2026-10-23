/**
 * Sizes in the units people read them in.
 *
 * One definition rather than one per component: two copies drift, and a
 * message that says 2 KB in the composer and 2.0 KB in the reading pane reads
 * as two different numbers for the same file.
 *
 * Deliberately coarse. The number answers "is this big" and nothing finer, so
 * a byte-exact figure would be precision the reader has no use for.
 */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
