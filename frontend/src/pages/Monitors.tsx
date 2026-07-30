import { AlertTriangle, Plus, RefreshCw, Target } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import MonitorCard from '@/components/MonitorCard'
import MonitorEditor from '@/components/MonitorEditor'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { parseISO, relativeTime } from '@/lib/time'
import { GetMonitor, ListMonitors, RefreshMonitors } from '@wails/go/main/App'
import { type main, type monitor } from '@wails/go/models'
import { EventsOff, EventsOn } from '@wails/runtime/runtime'

const EVENT_MONITORS = 'monitors:updated'

export default function Monitors() {
  const [result, setResult] = useState<main.MonitorsResult | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<monitor.Monitor | null>(null)

  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    setRefreshing(true)
    try {
      setResult(await RefreshMonitors())
    } finally {
      inFlight.current = false
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    void (async () => {
      const cached = await ListMonitors()
      if (!cancelled) setResult(cached)
    })()

    return () => {
      cancelled = true
    }
  }, [])

  // Driven by the billing poller on the Go side, same as the billing bar.
  useEffect(() => {
    EventsOn(EVENT_MONITORS, (payload: main.MonitorsResult) =>
      setResult(payload)
    )

    return () => {
      EventsOff(EVENT_MONITORS)
    }
  }, [])

  async function openEditor(id: number | null) {
    setEditing(id === null ? null : await GetMonitor(id))
    setEditorOpen(true)
  }

  const list = result?.monitors ?? []
  const loading = result === null

  return (
    <div className="mx-auto w-full max-w-3xl p-6">
      <div className="mb-4 flex items-end justify-between">
        <div>
          <h1 className="text-lg font-medium">Targets</h1>
          <p className="text-muted-foreground text-xs">
            {result?.syncedAt
              ? `Updated ${relativeTime(parseISO(result.syncedAt))}`
              : 'Not synced yet'}
          </p>
        </div>

        <div className="flex items-center gap-2">
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
          <Button size="sm" onClick={() => void openEditor(null)}>
            <Plus className="size-4" />
            New monitor
          </Button>
        </div>
      </div>

      {result?.errorMessage && (
        <Alert variant="destructive" className="mb-4">
          <AlertTriangle className="size-4" />
          <AlertDescription>
            {result.fromCacheOnly
              ? `Showing cached figures — refresh failed: ${result.errorMessage}`
              : result.errorMessage}
          </AlertDescription>
        </Alert>
      )}

      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-36 w-full" />
          <Skeleton className="h-36 w-full" />
        </div>
      ) : list.length === 0 ? (
        <div className="text-muted-foreground rounded-lg border border-dashed py-16 text-center">
          <Target className="mx-auto mb-3 size-8 opacity-40" />
          <p className="text-sm">No targets yet.</p>
          <p className="mt-1 text-xs">
            Create a monitor to track hours for a group of projects and people.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {list.map((progress) => (
            <MonitorCard
              key={progress.monitorId}
              progress={progress}
              onEdit={() => void openEditor(progress.monitorId)}
            />
          ))}
        </div>
      )}

      <MonitorEditor
        open={editorOpen}
        onOpenChange={setEditorOpen}
        existing={editing}
        onSaved={() => void refresh()}
      />
    </div>
  )
}
