import { dayLabel, formatHours, isToday } from '@/lib/time'
import { type team } from '@wails/go/models'

type Props = {
  view: team.BoardView
}

/**
 * One row per person: today, yesterday and the week, with the daily breakdown
 * alongside.
 *
 * Built from a grid rather than a table because shadcn's table component is not
 * vendored here, and this matches the card look the rest of the app uses.
 * Every duration goes through formatHours — minutes in, h:mm out, never a
 * decimal.
 */
export default function TeamBillingBoard({ view }: Props) {
  if (view.rows.length === 0) {
    return (
      <div className="text-muted-foreground rounded-lg border border-dashed py-12 text-center text-sm">
        No people on this board yet.
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <div className="min-w-[44rem] space-y-2">
        <div className="text-muted-foreground grid grid-cols-[minmax(9rem,1fr)_5rem_5rem_5rem_auto] items-baseline gap-3 px-3 text-xs">
          <span>Person</span>
          <span className="text-right">Today</span>
          <span className="text-right">Yesterday</span>
          <span className="text-right">Week</span>
          <span className="pl-3">
            {view.weekStart} – {view.weekEnd}
          </span>
        </div>

        {view.rows.map((row) => (
          <div
            key={row.empId}
            className="bg-card grid grid-cols-[minmax(9rem,1fr)_5rem_5rem_5rem_auto] items-center gap-3 rounded-lg border px-3 py-2.5"
          >
            <span className="min-w-0 truncate text-sm font-medium">
              {row.empName}
            </span>
            <span className="text-right text-sm font-medium tabular-nums">
              {formatHours(row.todayMinutes)}
            </span>
            <span className="text-muted-foreground text-right text-sm tabular-nums">
              {formatHours(row.yesterdayMinutes)}
            </span>
            <span className="text-right text-sm tabular-nums">
              {formatHours(row.weekMinutes)}
            </span>

            <div className="flex gap-1 border-l pl-3">
              {row.days.map((day) => (
                <Day key={day.day} day={day} />
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function Day({ day }: { day: team.BoardView['rows'][number]['days'][number] }) {
  const minutes = day.billableMinutes + day.nonBillableMinutes
  const today = isToday(day.day)

  return (
    <div
      className={`w-14 rounded px-1 py-1 text-center ${
        today ? 'bg-accent' : ''
      }`}
      title={`${dayLabel(day.day)} · ${formatHours(day.billableMinutes)} billable, ${formatHours(day.nonBillableMinutes)} non-billable`}
    >
      <p className="text-muted-foreground text-[10px]">{dayLabel(day.day)}</p>
      <p
        className={`text-xs tabular-nums ${
          minutes === 0 ? 'text-muted-foreground/50' : ''
        }`}
      >
        {formatHours(minutes)}
      </p>
    </div>
  )
}
