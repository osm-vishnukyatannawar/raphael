import { AlertTriangle, Check, RefreshCw } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import TaskRow, { type RowTask } from '@/components/TaskRow'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { parseISO, relativeTime } from '@/lib/time'
import {
  ListTasks,
  MarkTasksSeen,
  OpenTask,
  RefreshTasks,
} from '@wails/go/main/App'
import { main, type settings } from '@wails/go/models'
import { EventsOff, EventsOn } from '@wails/runtime/runtime'

/** Emitted by the Go poller — see internal/poller for why it lives there. */
const EVENT_TASKS = 'tasks:updated'

type Props = {
  /** Read-only here; the settings dialog lives in the shell. */
  prefs: settings.Settings | null
}

export default function Tasks({ prefs }: Props) {
  const [result, setResult] = useState<main.TasksResult | null>(null)
  const [refreshing, setRefreshing] = useState(false)

  // Held in a ref so a manual refresh can't stack on top of itself, without
  // making the callback depend on the spinner state.
  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    setRefreshing(true)
    try {
      setResult(await RefreshTasks())
    } finally {
      inFlight.current = false
      setRefreshing(false)
    }
  }, [])

  // First paint: cached rows immediately, no network. The Go poller fires its
  // own refresh on startup, and the result arrives as the event below.
  useEffect(() => {
    let cancelled = false

    void (async () => {
      const cached = await ListTasks()
      if (!cancelled) setResult(cached)
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

    return () => {
      EventsOff(EVENT_TASKS)
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

  function open(task: RowTask) {
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
    <section className="flex min-h-0 flex-col">
      <div className="mb-3 flex items-end justify-between gap-2">
        <div>
          <h2 className="flex items-center gap-2 text-base font-medium">
            In review{!loading && ` (${list.length})`}
            {newCount > 0 && <Badge>{newCount} new</Badge>}
          </h2>
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
        <Alert variant="destructive" className="mb-3">
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
    </section>
  )
}
