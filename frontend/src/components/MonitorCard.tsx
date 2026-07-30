import { ChevronDown, Pencil, TrendingDown, TrendingUp } from 'lucide-react'
import { useState } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { formatHours } from '@/lib/time'
import { type monitor } from '@wails/go/models'

type Props = {
  progress: monitor.Progress
  onEdit: () => void
}

export default function MonitorCard({ progress, onEdit }: Props) {
  const [expanded, setExpanded] = useState(false)

  return (
    <section className="bg-card rounded-lg border">
      <div className="flex items-start justify-between gap-4 p-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 className="truncate font-medium">{progress.name}</h2>
            <PaceBadge progress={progress} />
          </div>
          <p className="text-muted-foreground mt-0.5 truncate text-xs">
            {progress.projects.map((p) => p.code).join(' · ') || 'No projects'}
            {' — '}
            {progress.periodStart} to {progress.periodEnd}
          </p>
        </div>

        <div className="flex shrink-0 items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            title="Edit monitor"
            onClick={onEdit}
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            title={expanded ? 'Hide the breakdown' : 'Show the breakdown'}
            aria-expanded={expanded}
            onClick={() => setExpanded((open) => !open)}
          >
            <ChevronDown
              className={`size-4 transition-transform ${expanded ? 'rotate-180' : ''}`}
            />
          </Button>
        </div>
      </div>

      <div className="space-y-2 px-4 pb-4">
        <div className="flex items-baseline justify-between gap-3">
          <p className="text-2xl font-semibold tabular-nums">
            {formatHours(progress.billableMinutes)}
            <span className="text-muted-foreground ml-1 text-sm font-normal">
              of {formatHours(progress.targetMinutes)}
            </span>
          </p>
          <p className="text-muted-foreground text-right text-xs">
            {catchUpLine(progress)}
          </p>
        </div>

        <TargetBar
          billableMinutes={progress.billableMinutes}
          targetMinutes={progress.targetMinutes}
          expectedByNowMinutes={progress.expectedByNowMinutes}
        />

        {progress.nonBillableMinutes > 0 && (
          <p className="text-muted-foreground text-xs">
            Plus {formatHours(progress.nonBillableMinutes)} non-billable, which
            does not count toward the target.
          </p>
        )}
      </div>

      {expanded && (
        <ul className="divide-y border-t">
          {progress.rows.length === 0 ? (
            <li className="text-muted-foreground p-4 text-center text-sm">
              No targets set yet.
            </li>
          ) : (
            progress.rows.map((row) => (
              <li
                key={`${row.empId}-${row.projectId}`}
                className="space-y-1.5 px-4 py-3"
              >
                <div className="flex items-baseline justify-between gap-3">
                  <span className="min-w-0 truncate text-sm">
                    {row.empName}
                    {/* projectId 0 means the target spans every project. */}
                    {row.projectId !== 0 && row.projectName && (
                      <span className="text-muted-foreground">
                        {' '}
                        · {row.projectName}
                      </span>
                    )}
                  </span>
                  <span className="shrink-0 text-sm tabular-nums">
                    {formatHours(row.billableMinutes)}
                    <span className="text-muted-foreground">
                      {' / '}
                      {formatHours(row.targetMinutes)}
                    </span>
                  </span>
                </div>

                <TargetBar
                  billableMinutes={row.billableMinutes}
                  targetMinutes={row.targetMinutes}
                  expectedByNowMinutes={row.expectedByNowMinutes}
                  compact
                />

                <p className="text-muted-foreground text-xs">
                  {catchUpLine(row)}
                </p>
              </li>
            ))
          )}
        </ul>
      )}
    </section>
  )
}

/**
 * Progress toward the target, with a marker for where pace says you should be.
 *
 * The marker is what makes the bar readable early in the month — 20% filled on
 * the 3rd is fine, on the 25th it is not, and the bar alone can't tell you which.
 */
function TargetBar({
  billableMinutes,
  targetMinutes,
  expectedByNowMinutes,
  compact,
}: {
  billableMinutes: number
  targetMinutes: number
  expectedByNowMinutes: number
  compact?: boolean
}) {
  if (targetMinutes <= 0) {
    return null
  }

  const pct = Math.min(100, (billableMinutes / targetMinutes) * 100)
  const expectedPct = Math.min(
    100,
    (expectedByNowMinutes / targetMinutes) * 100
  )
  const behind = billableMinutes < expectedByNowMinutes

  return (
    <div
      className={`bg-muted relative w-full overflow-hidden rounded-full ${
        compact ? 'h-1.5' : 'h-2.5'
      }`}
    >
      <div
        className={`h-full rounded-full transition-all ${
          behind ? 'bg-amber-500' : 'bg-emerald-500'
        }`}
        style={{ width: `${pct}%` }}
      />
      <div
        aria-hidden
        title="Where pace says you should be"
        className="bg-foreground/60 absolute inset-y-0 w-0.5"
        style={{ left: `${expectedPct}%` }}
      />
    </div>
  )
}

function PaceBadge({ progress }: { progress: monitor.Progress }) {
  if (progress.targetMinutes <= 0) {
    return null
  }

  return progress.onTrack ? (
    <Badge className="shrink-0 border-emerald-600/30 bg-emerald-600/15 text-emerald-700 dark:text-emerald-400">
      <TrendingUp className="size-3" />
      On track
    </Badge>
  ) : (
    <Badge className="shrink-0 border-amber-600/30 bg-amber-600/15 text-amber-700 dark:text-amber-400">
      <TrendingDown className="size-3" />
      Behind
    </Badge>
  )
}

/** The one line that answers "what do I have to do about it". */
function catchUpLine(p: {
  shortfallMinutes: number
  neededPerDayMinutes: number
  remainingWorkingDays?: number
  targetMinutes: number
}): string {
  if (p.targetMinutes <= 0) return 'No target set'
  if (p.shortfallMinutes <= 0) return 'Target met'

  const short = `${formatHours(p.shortfallMinutes)} short`

  // neededPerDay is 0 both when nothing is owed and when no days remain; the
  // shortfall check above means only the latter can reach here.
  if (p.neededPerDayMinutes <= 0) {
    return `${short} · no working days left`
  }

  const days = p.remainingWorkingDays
  const over =
    days === undefined
      ? ''
      : ` over ${days} working day${days === 1 ? '' : 's'}`

  return `${short} · ${formatHours(p.neededPerDayMinutes)}/day${over}`
}
