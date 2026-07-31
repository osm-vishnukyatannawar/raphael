import {
  AlertTriangle,
  CalendarSearch,
  ChevronDown,
  RefreshCw,
} from 'lucide-react'
import { useState } from 'react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  dayLabel,
  formatHours,
  isToday,
  parseISO,
  relativeTime,
  toDayKey,
} from '@/lib/time'
import { GetBillingForDate } from '@wails/go/main/App'
import { type billing, type main } from '@wails/go/models'

type Props = {
  result: main.BillingResult | null
  refreshing: boolean
  onRefresh: () => void
}

export default function BillingBar({ result, refreshing, onRefresh }: Props) {
  const [expanded, setExpanded] = useState(false)

  const summary = result?.summary ?? null

  return (
    <section className="bg-card mb-4 rounded-lg border">
      <div className="flex items-center justify-between gap-4 px-4 py-3">
        {summary ? (
          <div className="flex items-baseline gap-5">
            <Figure label="Today" minutes={summary.todayMinutes} emphasis />
            <Figure label="This week" minutes={summary.weekMinutes} />
            <Figure
              label="Yesterday"
              minutes={summary.yesterdayMinutes}
              muted
            />
          </div>
        ) : (
          <Skeleton className="h-9 w-72" />
        )}

        <div className="flex items-center gap-1">
          <span className="text-muted-foreground mr-1 hidden text-xs sm:inline">
            {result?.syncedAt
              ? relativeTime(parseISO(result.syncedAt))
              : 'not synced'}
          </span>
          <Button
            variant="ghost"
            size="icon"
            title="Refresh billing"
            disabled={refreshing}
            onClick={onRefresh}
          >
            <RefreshCw
              className={`size-4 ${refreshing ? 'animate-spin' : ''}`}
            />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            title={
              expanded ? 'Hide the daily breakdown' : 'Show the daily breakdown'
            }
            aria-expanded={expanded}
            onClick={() => setExpanded((open) => !open)}
          >
            <ChevronDown
              className={`size-4 transition-transform ${expanded ? 'rotate-180' : ''}`}
            />
          </Button>
        </div>
      </div>

      {/* A failed refresh warns without discarding the cached figures above. */}
      {result?.errorMessage && (
        <div className="px-4 pb-3">
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertDescription>
              {result.fromCacheOnly
                ? `Showing cached hours — refresh failed: ${result.errorMessage}`
                : result.errorMessage}
            </AlertDescription>
          </Alert>
        </div>
      )}

      {expanded && summary && (
        <div className="space-y-4 border-t px-4 py-3">
          <WeekGrid summary={summary} />
          <DateLookup />
        </div>
      )}
    </section>
  )
}

function Figure({
  label,
  minutes,
  emphasis,
  muted,
}: {
  label: string
  minutes: number
  emphasis?: boolean
  muted?: boolean
}) {
  return (
    <div className={muted ? 'hidden sm:block' : ''}>
      <p className="text-muted-foreground text-xs">{label}</p>
      <p
        className={
          emphasis
            ? 'text-xl font-semibold tabular-nums'
            : `font-medium tabular-nums ${muted ? 'text-muted-foreground text-sm' : ''}`
        }
      >
        {formatHours(minutes)}
      </p>
    </div>
  )
}

function WeekGrid({ summary }: { summary: billing.Summary }) {
  // The Go side always returns seven zero-filled days, so this never has gaps.
  const busiest = Math.max(1, ...summary.days.map((d) => dayMinutes(d)))

  return (
    <div>
      <p className="text-muted-foreground mb-2 text-xs">
        {summary.weekStart} — {summary.weekEnd}
      </p>
      <ul className="grid grid-cols-7 gap-1">
        {summary.days.map((day) => {
          const minutes = dayMinutes(day)
          const today = isToday(day.day)

          return (
            <li key={day.day} className="flex flex-col items-center gap-1">
              {/* A bar rather than a number alone: the shape of the week is
                  readable at a glance, the exact figure is underneath. */}
              <div className="bg-muted flex h-16 w-full items-end rounded">
                <div
                  className={`w-full rounded transition-all ${
                    today ? 'bg-primary' : 'bg-primary/40'
                  }`}
                  style={{
                    height: `${Math.round((minutes / busiest) * 100)}%`,
                  }}
                />
              </div>
              <span
                className={`text-[10px] ${today ? 'font-medium' : 'text-muted-foreground'}`}
              >
                {dayLabel(day.day)}
              </span>
              <span className="text-[10px] tabular-nums">
                {minutes === 0 ? '—' : formatHours(minutes)}
              </span>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

function dayMinutes(day: billing.DayTotal): number {
  return day.billableMinutes + day.nonBillableMinutes
}

function DateLookup() {
  const [day, setDay] = useState(() => toDayKey(new Date()))
  const [busy, setBusy] = useState(false)
  const [total, setTotal] = useState<billing.DayTotal | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function look() {
    setBusy(true)
    setError(null)
    setTotal(null)
    try {
      setTotal(await GetBillingForDate(day))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="border-t pt-3">
      <Label htmlFor="billing-date" className="text-xs">
        Hours on a specific date
      </Label>
      <div className="mt-2 flex items-center gap-2">
        <Input
          id="billing-date"
          type="date"
          value={day}
          onChange={(e) => setDay(e.target.value)}
          className="w-44"
        />
        <Button
          variant="outline"
          size="sm"
          disabled={busy || !day}
          onClick={() => void look()}
        >
          <CalendarSearch className="size-4" />
          {busy ? 'Checking…' : 'Check'}
        </Button>

        {total && (
          <span className="text-sm">
            <span className="font-medium tabular-nums">
              {formatHours(total.billableMinutes + total.nonBillableMinutes)}
            </span>
            {total.nonBillableMinutes > 0 && (
              <span className="text-muted-foreground">
                {' '}
                ({formatHours(total.nonBillableMinutes)} non-billable)
              </span>
            )}
          </span>
        )}
        {error && <span className="text-destructive text-xs">{error}</span>}
      </div>
    </div>
  )
}
