import {
  AlertTriangle,
  Building2,
  ExternalLink,
  LogOut,
  RefreshCw,
  Settings as SettingsIcon,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import SettingsDialog from '@/components/SettingsDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
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
  GetSettings,
  ListTasks,
  OpenTask,
  RefreshTasks,
  SignOut,
} from '@wails/go/main/App'
import {
  type identity,
  type main,
  type settings,
  type tasks,
} from '@wails/go/models'

type Props = {
  session: identity.Session
  onSignedOut: () => void
}

function greeting(now = new Date()): string {
  const hour = now.getHours()
  if (hour < 12) return 'Good morning'
  if (hour < 18) return 'Good afternoon'
  return 'Good evening'
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  const first = parts[0][0]
  const last = parts.length > 1 ? parts[parts.length - 1][0] : ''

  return (first + last).toUpperCase()
}

export default function Tasks({ session, onSignedOut }: Props) {
  const [result, setResult] = useState<main.TasksResult | null>(null)
  const [prefs, setPrefs] = useState<settings.Settings | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)

  // Held in a ref so the interval effect doesn't need `refreshing` as a
  // dependency — that would tear down and re-arm the timer on every refresh.
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

  // First paint: cached rows immediately (no network), then a live refresh.
  useEffect(() => {
    let cancelled = false

    void (async () => {
      const [cached, loadedPrefs] = await Promise.all([
        ListTasks(),
        GetSettings(),
      ])
      if (cancelled) return
      setResult(cached)
      setPrefs(loadedPrefs)
      void refresh()
    })()

    return () => {
      cancelled = true
    }
  }, [refresh])

  // Auto-refresh. Re-armed whenever the interval changes; 0 disables it.
  useEffect(() => {
    const seconds = prefs?.refreshIntervalSeconds ?? 0
    if (seconds <= 0) return

    const id = setInterval(() => void refresh(), seconds * 1000)

    return () => clearInterval(id)
  }, [prefs?.refreshIntervalSeconds, refresh])

  const list = result?.tasks ?? []
  const loading = result === null

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between border-b px-6 py-3">
        <div className="flex items-center gap-3">
          <Avatar className="size-8">
            <AvatarFallback className="text-xs">
              {initials(session.displayName)}
            </AvatarFallback>
          </Avatar>
          <div className="leading-tight">
            <p className="text-sm font-medium">
              {greeting()}, {session.displayName.split(' ')[0]}
            </p>
            <p className="text-muted-foreground flex items-center gap-1 text-xs">
              <Building2 className="size-3" />
              {session.companyName}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            title="Settings"
            onClick={() => setSettingsOpen(true)}
          >
            <SettingsIcon className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void SignOut().then(onSignedOut)}
          >
            <LogOut className="size-4" />
            Sign out
          </Button>
        </div>
      </header>

      <main className="mx-auto w-full max-w-3xl flex-1 overflow-y-auto p-6">
        <div className="mb-4 flex items-end justify-between">
          <div>
            <h1 className="text-lg font-medium">
              In review{!loading && ` (${list.length})`}
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
              <TaskRow key={task.taskId} task={task} />
            ))}
          </ul>
        )}
      </main>

      {prefs && (
        <SettingsDialog
          open={settingsOpen}
          onOpenChange={setSettingsOpen}
          current={prefs}
          onSaved={setPrefs}
        />
      )}
    </div>
  )
}

function TaskRow({ task }: { task: tasks.Task }) {
  const due = parsePinestemDate(task.dueDate)
  const modified = parsePinestemDate(task.modifiedOn)
  const overdue = isOverdue(due)

  return (
    <li>
      <button
        type="button"
        onClick={() => void OpenTask(task.shortCode)}
        className="hover:bg-accent/50 group flex w-full items-start gap-3 px-4 py-3 text-left transition-colors"
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
