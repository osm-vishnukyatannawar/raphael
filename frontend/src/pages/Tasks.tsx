import { AlertTriangle, Check, ExternalLink, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import BillingBar from '@/components/BillingBar'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatDate,
  isOverdue,
  parseISO,
  parsePinestemDate,
  relativeTime,
} from '@/lib/time'
import {
  GetBilling,
  ListTasks,
  MarkTasksSeen,
  OpenTask,
  RefreshBilling,
  RefreshTasks,
} from '@wails/go/main/App'
import { main, type settings, type tasks } from '@wails/go/models'
import { EventsOff, EventsOn } from '@wails/runtime/runtime'

/** Emitted by the Go pollers — see internal/poller for why they live there. */
const EVENT_TASKS = 'tasks:updated'
const EVENT_BILLING = 'billing:updated'

type Props = {
  /** Read-only here; the settings dialog lives in the shell. */
  prefs: settings.Settings | null
}

export default function Tasks({ prefs }: Props) {
  const [result, setResult] = useState<main.TasksResult | null>(null)
  const [billing, setBilling] = useState<main.BillingResult | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [refreshingBilling, setRefreshingBilling] = useState(false)

  // Held in refs so a manual refresh can't stack on top of itself, without
  // making the callbacks depend on the spinner state.
  const tasksInFlight = useRef(false)
  const billingInFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (tasksInFlight.current) return
    tasksInFlight.current = true
    setRefreshing(true)
    try {
      setResult(await RefreshTasks())
    } finally {
      tasksInFlight.current = false
      setRefreshing(false)
    }
  }, [])

  const refreshBilling = useCallback(async () => {
    if (billingInFlight.current) return
    billingInFlight.current = true
    setRefreshingBilling(true)
    try {
      setBilling(await RefreshBilling())
    } finally {
      billingInFlight.current = false
      setRefreshingBilling(false)
    }
  }, [])

  // First paint: cached rows immediately, no network. The Go pollers fire their
  // own refresh on startup, and the results arrive as events below.
  useEffect(() => {
    let cancelled = false

    void (async () => {
      const [cachedTasks, cachedBilling] = await Promise.all([
        ListTasks(),
        GetBilling(),
      ])
      if (cancelled) return
      setResult(cachedTasks)
      setBilling(cachedBilling)
    })()

    return () => {
      cancelled = true
    }
  }, [])

  // Backend-driven refreshes. There is no setInterval here on purpose: a hidden
  // page has its timers throttled, and the new-task alert has to work when this
  // window is not in front.
  useEffect(() => {
    EventsOn(EVENT_TASKS, (payload: main.TasksResult) => setResult(payload))
    EventsOn(EVENT_BILLING, (payload: main.BillingResult) =>
      setBilling(payload)
    )

    return () => {
      EventsOff(EVENT_TASKS)
      EventsOff(EVENT_BILLING)
    }
  }, [])

  const list = result?.tasks ?? []
  const loading = result === null
  const newCount = list.filter((task) => task.isNew).length

  async function markSeen() {
    await MarkTasksSeen()
    // createFrom rather than a spread: the generated classes carry a
    // convertValues method, so a plain object literal is not a TasksResult.
    setResult((prev) =>
      prev
        ? main.TasksResult.createFrom({
            ...prev,
            tasks: prev.tasks.map((task) => ({ ...task, isNew: false })),
          })
        : prev
    )
  }

  function open(task: tasks.Task) {
    void OpenTask(task.taskId, task.shortCode)
    // Opening a task is what "seen" means; clear it here too so the highlight
    // goes immediately rather than at the next refresh.
    setResult((prev) =>
      prev
        ? main.TasksResult.createFrom({
            ...prev,
            tasks: prev.tasks.map((t) =>
              t.taskId === task.taskId ? { ...t, isNew: false } : t
            ),
          })
        : prev
    )
  }

  return (
    <main className="mx-auto w-full max-w-3xl p-6">
      <BillingBar
        result={billing}
        refreshing={refreshingBilling}
        onRefresh={() => void refreshBilling()}
      />

      <div className="mb-4 flex items-end justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-medium">
            In review{!loading && ` (${list.length})`}
            {newCount > 0 && <Badge>{newCount} new</Badge>}
          </h1>
          <p className="text-muted-foreground text-xs">
            {result?.syncedAt
              ? `Updated ${relativeTime(parseISO(result.syncedAt))}`
              : 'Not synced yet'}
            {prefs && prefs.refreshIntervalSeconds > 0
              ? ` · auto every ${prefs.refreshIntervalSeconds}s`
              : ' · auto-refresh off'}
          </p>
        </div>

        <div className="flex items-center gap-2">
          {/* Only offered when there is something to clear. */}
          {newCount > 0 && (
            <Button variant="ghost" size="sm" onClick={() => void markSeen()}>
              <Check className="size-4" />
              Mark all seen
            </Button>
          )}
          <Button
            variant="outline"
            size="sm"
            disabled={refreshing}
            onClick={() => void refresh()}
          >
            <RefreshCw
              className={`size-4 ${refreshing ? 'animate-spin' : ''}`}
            />
            Refresh
          </Button>
        </div>
      </div>

      {/* A failed refresh warns without discarding the cached rows below. */}
      {result?.errorMessage && (
        <Alert variant="destructive" className="mb-4">
          <AlertTriangle className="size-4" />
          <AlertDescription>
            {result.fromCacheOnly
              ? `Showing cached tasks — refresh failed: ${result.errorMessage}`
              : result.errorMessage}
          </AlertDescription>
        </Alert>
      )}

      {loading ? (
        <div className="space-y-2">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : list.length === 0 ? (
        <div className="text-muted-foreground rounded-lg border border-dashed py-16 text-center text-sm">
          Nothing in review right now.
        </div>
      ) : (
        <ul className="divide-y rounded-lg border">
          {list.map((task) => (
            <TaskRow key={task.taskId} task={task} onOpen={open} />
          ))}
        </ul>
      )}
    </main>
  )
}

function TaskRow({
  task,
  onOpen,
}: {
  task: tasks.Task
  onOpen: (task: tasks.Task) => void
}) {
  const due = parsePinestemDate(task.dueDate)
  const modified = parsePinestemDate(task.modifiedOn)
  const overdue = isOverdue(due)

  return (
    <li>
      <button
        type="button"
        onClick={() => onOpen(task)}
        className={`group flex w-full items-start gap-3 border-l-2 px-4 py-3 text-left transition-colors ${
          task.isNew
            ? 'border-l-primary bg-primary/5 hover:bg-primary/10'
            : 'hover:bg-accent/50 border-l-transparent'
        }`}
      >
        <span
          aria-hidden
          className="mt-1.5 size-2 shrink-0 rounded-full"
          style={{
            backgroundColor: task.statusColor || 'var(--muted-foreground)',
          }}
        />

        <div className="min-w-0 flex-1">
          {/*
            shrink-0 on the code stops flex from squeezing "REST-2408" onto two
            lines when the title is long; min-w-0 on the row is what lets the
            title actually truncate inside a nested flex container.
          */}
          <div className="flex min-w-0 items-baseline gap-2">
            <span className="shrink-0 font-mono text-xs font-medium whitespace-nowrap">
              {task.shortCode}
            </span>
            <span className="truncate text-sm">{task.name}</span>
            {task.isNew && (
              <Badge variant="default" className="shrink-0 text-[10px]">
                New
              </Badge>
            )}
          </div>

          <p className="text-muted-foreground mt-1 text-xs">
            {task.projectName}
            {task.priority && ` · ${task.priority}`}
            {due && (
              <span className={overdue ? 'text-destructive font-medium' : ''}>
                {' · due '}
                {formatDate(due)}
              </span>
            )}
            {modified && ` · modified ${relativeTime(modified)}`}
          </p>
        </div>

        <ExternalLink className="text-muted-foreground mt-1 size-3.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
      </button>
    </li>
  )
}
