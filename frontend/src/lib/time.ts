/**
 * Pinestem timestamp helpers.
 *
 * Pinestem returns "2026-07-28 23:01:42" — a space separator, no timezone.
 * `new Date(...)` on that string is implementation-defined: V8 accepts it,
 * other engines may return Invalid Date. Raphael runs on WebKitGTK (Linux) and
 * WebView2/Chromium (Windows), so it is parsed explicitly rather than trusted
 * to the engine.
 */

const PINESTEM_TIMESTAMP = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2}):(\d{2})/

/** Parses a Pinestem timestamp as local time. Returns null if unparseable. */
export function parsePinestemDate(value: string | undefined): Date | null {
  if (!value) return null

  const m = PINESTEM_TIMESTAMP.exec(value)
  if (!m) return null

  const [, year, month, day, hour, minute, second] = m
  const date = new Date(
    Number(year),
    Number(month) - 1,
    Number(day),
    Number(hour),
    Number(minute),
    Number(second)
  )

  // Pinestem uses year 1 as its null sentinel.
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 1900) return null

  return date
}

/** ISO-8601 (the Go side's sync timestamps). Returns null if unparseable. */
export function parseISO(value: string | undefined): Date | null {
  if (!value) return null

  const date = new Date(value)

  return Number.isNaN(date.getTime()) ? null : date
}

const UNITS: [limitSeconds: number, perUnit: number, label: string][] = [
  [60, 1, 'second'],
  [3600, 60, 'minute'],
  [86400, 3600, 'hour'],
  [2592000, 86400, 'day'],
]

/** "just now", "5 minutes ago", "2 days ago"; absolute date beyond a month. */
export function relativeTime(date: Date | null, now = new Date()): string {
  if (!date) return ''

  const seconds = Math.round((now.getTime() - date.getTime()) / 1000)
  if (seconds < 45) return 'just now'

  // Future timestamps happen when the machine clock lags the server.
  if (seconds < 0) return 'just now'

  for (const [limit, perUnit, label] of UNITS) {
    if (seconds < limit) {
      const value = Math.floor(seconds / perUnit)

      return `${value} ${label}${value === 1 ? '' : 's'} ago`
    }
  }

  return formatDate(date)
}

/** "30 Jul" for the current year, "30 Jul 2025" otherwise. */
export function formatDate(date: Date | null, now = new Date()): string {
  if (!date) return ''

  const sameYear = date.getFullYear() === now.getFullYear()

  return date.toLocaleDateString(undefined, {
    day: 'numeric',
    month: 'short',
    ...(sameYear ? {} : { year: 'numeric' }),
  })
}

/** True when a due date has passed, for highlighting overdue work. */
export function isOverdue(date: Date | null, now = new Date()): boolean {
  if (!date) return false

  // Due dates arrive at midnight, so the task is late only once that day is over.
  const endOfDueDay = new Date(
    date.getFullYear(),
    date.getMonth(),
    date.getDate(),
    23,
    59,
    59
  )

  return endOfDueDay.getTime() < now.getTime()
}

/**
 * "4.50 h" from Pinestem's integer minutes.
 *
 * Two decimals matches how Pinestem itself renders hours (its own
 * `*_HoursFormat` fields), so numbers here can be compared against the web app
 * without mental arithmetic.
 */
export function formatHours(minutes: number): string {
  return `${(minutes / 60).toFixed(2)} h`
}

/** Parses a "YYYY-MM-DD" day key as a local date. Null if unparseable. */
export function parseDay(day: string | undefined): Date | null {
  if (!day) return null

  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(day)
  if (!m) return null

  const [, year, month, date] = m

  return new Date(Number(year), Number(month) - 1, Number(date))
}

/** "Mon 27" for a day key, for the week breakdown. */
export function dayLabel(day: string): string {
  const date = parseDay(day)
  if (!date) return day

  return date.toLocaleDateString(undefined, {
    weekday: 'short',
    day: 'numeric',
  })
}

/** "YYYY-MM-DD" for a Date, in local time — the inverse of parseDay. */
export function toDayKey(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

/** True when the day key is today, for emphasising it in the week grid. */
export function isToday(day: string, now = new Date()): boolean {
  return day === toDayKey(now)
}
