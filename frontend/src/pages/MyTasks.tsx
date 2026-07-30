import {
  AlertTriangle,
  Check,
  Eye,
  RefreshCw,
  SlidersHorizontal,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import MyTasksFilterDialog from '@/components/MyTasksFilterDialog'
import TaskRow, { type RowTask } from '@/components/TaskRow'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { parseISO, relativeTime } from '@/lib/time'
import {
  HideMyTask,
  ListMyTasks,
  MarkMyTasksSeen,
  OpenTask,
  RefreshMyTasks,
  UnhideAllMyTasks,
  UnhideMyTask,
} from '@wails/go/main/App'
import { main, type settings } from '@wails/go/models'
import { EventsOff, EventsOn } from '@wails/runtime/runtime'

const EVENT_MY_TASKS = 'mytasks:updated'

type Props = {
  prefs: settings.Settings | null
}

/**
 * Every task assigned to the user, filtered by project and status.
 *
 * Ordered by due date rather than modification time — the opposite of the
 * review queue. This list is a backlog, and what is nearly due matters more
 * than what someone touched five minutes ago. The Go side does the sorting.
 */
export default function MyTasks({ prefs }: Props) {
  const [result, setResult] = useState<main.MyTasksResult | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [filterOpen, setFilterOpen] = useState(false)
  const [showHidden, setShowHidden] = useState(false)

  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    setRefreshing(true)
    try {
      setResult(await RefreshMyTasks())
    } finally {
      inFlight.current = false
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    void (async () => {
      const cached = await ListMyTasks()
      if (!cancelled) setResult(cached)
    })()

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    EventsOn(EVENT_MY_TASKS, (payload: main.MyTasksResult) =>
      setResult(payload)
    )

    return () => {
      EventsOff(EVENT_MY_TASKS)
    }
  }, [])

  const all = result?.tasks ?? []
  const hiddenList = result?.hidden ?? []
  const visible = all.filter((task) => !task.hidden)
  const loading = result === null
  // Hidden tasks never alert, so they must not be counted here either.
  const newCount = visible.filter((task) => task.isNew).length

  function patch(
    update: (task: main.MyTasksResult['tasks'][number]) => object
  ) {
    // createFrom rather than a spread: the generated classes carry a
    // convertValues method, so a plain object literal is not a MyTasksResult.
    setResult((prev) =>
      prev
        ? main.MyTasksResult.createFrom({
            ...prev,
            tasks: prev.tasks.map((task) => ({ ...task, ...update(task) })),
          })
        : prev
    )
  }

  async function markSeen() {
    await MarkMyTasksSeen()
    patch(() => ({ isNew: false }))
  }

  function open(task: RowTask) {
    void OpenTask(task.taskId, task.shortCode)
    patch((t) => (t.taskId === task.taskId ? { isNew: false } : {}))
  }

  async function hide(task: RowTask) {
    await HideMyTask(task.taskId)
    // Refresh rather than patching locally: the hide list is server-side state,
    // and it is what the "show hidden" panel below renders from.
    await refresh()
  }

  async function unhide(taskID: number) {
    await UnhideMyTask(taskID)
    await refresh()
  }

  async function unhideAll() {
    await UnhideAllMyTasks()
    await refresh()
  }

  return (
    <section className="flex min-h-0 flex-col">
      <div className="mb-3 flex items-end justify-between gap-2">
        <div>
          <h2 className="flex items-center gap-2 text-base font-medium">
            My tasks{!loading && ` (${visible.length})`}
            {newCount > 0 && <Badge>{newCount} new</Badge>}
          </h2>
          <p className="text-muted-foreground text-xs">
            {result?.syncedAt
              ? `Updated ${relativeTime(parseISO(result.syncedAt))}`
              : 'Not synced yet'}
            {prefs && prefs.myTasksRefreshIntervalSeconds > 0
              ? ` · auto every ${prefs.myTasksRefreshIntervalSeconds}s`
              : ' · auto-refresh off'}
          </p>
        </div>

        <div className="flex items-center gap-2">
          {newCount > 0 && (
            <Button variant="ghost" size="sm" onClick={() => void markSeen()}>
              <Check className="size-4" />
              Mark all seen
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            title="Filter projects and statuses"
            onClick={() => setFilterOpen(true)}
          >
            <SlidersHorizontal className="size-4" />
            Filter
          </Button>
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
      ) : visible.length === 0 ? (
        <div className="text-muted-foreground rounded-lg border border-dashed py-16 text-center text-sm">
          Nothing assigned in the current filter.
        </div>
      ) : (
        <ul className="divide-y rounded-lg border">
          {visible.map((task) => (
            <TaskRow
              key={task.taskId}
              task={task}
              onOpen={open}
              showStatus
              onHide={(t) => void hide(t)}
            />
          ))}
        </ul>
      )}

      {hiddenList.length > 0 && (
        <div className="mt-3">
          <div className="flex items-center justify-between">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowHidden((open) => !open)}
            >
              <Eye className="size-4" />
              {showHidden ? 'Hide' : 'Show'} hidden ({hiddenList.length})
            </Button>
            {showHidden && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => void unhideAll()}
              >
                Unhide all
              </Button>
            )}
          </div>

          {showHidden && (
            <ul className="mt-2 divide-y rounded-lg border border-dashed">
              {hiddenList.map((task) => (
                <li
                  key={task.taskId}
                  className="flex items-center gap-2 px-4 py-2"
                >
                  <span className="text-muted-foreground shrink-0 font-mono text-xs">
                    {task.shortCode || task.taskId}
                  </span>
                  <span className="text-muted-foreground min-w-0 flex-1 truncate text-sm">
                    {task.name || 'Hidden before it was first seen'}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => void unhide(task.taskId)}
                  >
                    Unhide
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}

      <MyTasksFilterDialog
        open={filterOpen}
        onOpenChange={setFilterOpen}
        onSaved={() => void refresh()}
      />
    </section>
  )
}
