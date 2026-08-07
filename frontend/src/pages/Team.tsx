import {
  AlertTriangle,
  CalendarClock,
  ListChecks,
  Plus,
  RefreshCw,
  SlidersHorizontal,
  Users,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'

import TeamBillingBoard from '@/components/TeamBillingBoard'
import TeamBoardEditor, {
  KIND_BILLING,
  KIND_TASKS,
} from '@/components/TeamBoardEditor'
import TeamTaskBoard from '@/components/TeamTaskBoard'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { parseISO, relativeTime } from '@/lib/time'
import {
  GetTeamBoard,
  ListTeamBoards,
  RefreshTeamBoards,
} from '@wails/go/main/App'
import { type main, type settings, type team } from '@wails/go/models'
import { EventsOff, EventsOn } from '@wails/runtime/runtime'

const EVENT_TEAM = 'team:updated'

type Props = {
  prefs: settings.Settings | null
}

/**
 * Named boards over other people's work.
 *
 * Boards live inside this one tab rather than each becoming a top-level tab:
 * there can be any number of them, and the header would degrade past a handful.
 */
export default function Team({ prefs }: Props) {
  const [result, setResult] = useState<main.TeamResult | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const [activeId, setActiveId] = useState<number | null>(null)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editing, setEditing] = useState<team.Board | null>(null)
  const [newKind, setNewKind] = useState<string>(KIND_TASKS)

  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    setRefreshing(true)
    try {
      setResult(await RefreshTeamBoards())
    } finally {
      inFlight.current = false
      setRefreshing(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    void (async () => {
      const cached = await ListTeamBoards()
      if (!cancelled) setResult(cached)
    })()

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    EventsOn(EVENT_TEAM, (payload: main.TeamResult) => setResult(payload))

    return () => {
      EventsOff(EVENT_TEAM)
    }
  }, [])

  const boards = result?.boards ?? []
  const loading = result === null

  // Fall back to the first board rather than storing a default, so deleting the
  // selected board can't leave the strip pointing at nothing.
  const active =
    boards.find((b) => b.board.id === activeId) ?? boards[0] ?? null

  async function openEditor(id: number | null, kind: string) {
    setNewKind(kind)
    setEditing(id === null ? null : await GetTeamBoard(id))
    setEditorOpen(true)
  }

  return (
    <div className="mx-auto w-full max-w-6xl p-6">
      <div className="mb-4 flex items-end justify-between gap-2">
        <div>
          <h1 className="text-lg font-medium">Team</h1>
          <p className="text-muted-foreground text-xs">
            {result?.syncedAt
              ? `Updated ${relativeTime(parseISO(result.syncedAt))}`
              : 'Not synced yet'}
            {prefs && prefs.teamRefreshIntervalSeconds > 0
              ? ` · auto every ${prefs.teamRefreshIntervalSeconds}s`
              : ' · auto-refresh off'}
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
          <Button size="sm" onClick={() => void openEditor(null, KIND_TASKS)}>
            <Plus className="size-4" />
            Task board
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={() => void openEditor(null, KIND_BILLING)}
          >
            <Plus className="size-4" />
            Hours board
          </Button>
        </div>
      </div>

      {result?.errorMessage && (
        <Alert variant="destructive" className="mb-4">
          <AlertTriangle className="size-4" />
          <AlertDescription>
            {result.fromCacheOnly
              ? `Showing cached boards — refresh failed: ${result.errorMessage}`
              : result.errorMessage}
          </AlertDescription>
        </Alert>
      )}

      {loading ? (
        <div className="space-y-3">
          <Skeleton className="h-9 w-64" />
          <Skeleton className="h-48 w-full" />
        </div>
      ) : boards.length === 0 ? (
        <div className="text-muted-foreground rounded-lg border border-dashed py-16 text-center">
          <Users className="mx-auto mb-3 size-8 opacity-40" />
          <p className="text-sm">No team boards yet.</p>
          <p className="mt-1 text-xs">
            A task board shows what a group of people have on; an hours board
            shows what they billed.
          </p>
        </div>
      ) : (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-1 border-b pb-2">
            {boards.map((view) => (
              <button
                key={view.board.id}
                type="button"
                onClick={() => setActiveId(view.board.id)}
                className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm transition-colors ${
                  view.board.id === active?.board.id
                    ? 'bg-accent text-accent-foreground font-medium'
                    : 'text-muted-foreground hover:bg-accent/50'
                }`}
              >
                {view.board.kind === KIND_TASKS ? (
                  <ListChecks className="size-3.5" />
                ) : (
                  <CalendarClock className="size-3.5" />
                )}
                {view.board.name}
              </button>
            ))}
          </div>

          {active && (
            <>
              <div className="mb-3 flex items-center justify-between gap-2">
                <p className="text-muted-foreground text-xs">
                  {active.board.projects.map((p) => p.code).join(', ') ||
                    'No projects'}
                  {' · '}
                  {active.board.members.length}{' '}
                  {active.board.members.length === 1 ? 'person' : 'people'}
                </p>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    void openEditor(active.board.id, active.board.kind)
                  }
                >
                  <SlidersHorizontal className="size-4" />
                  Edit board
                </Button>
              </div>

              {!active.configured ? (
                <div className="text-muted-foreground rounded-lg border border-dashed py-12 text-center text-sm">
                  This board needs projects
                  {active.board.kind === KIND_TASKS ? ', statuses' : ''} and
                  people before it can show anything.
                </div>
              ) : active.board.kind === KIND_TASKS ? (
                <TeamTaskBoard view={active} />
              ) : (
                <TeamBillingBoard view={active} />
              )}
            </>
          )}
        </>
      )}

      <TeamBoardEditor
        open={editorOpen}
        onOpenChange={setEditorOpen}
        existing={editing}
        kind={newKind}
        onSaved={() => void refresh()}
      />
    </div>
  )
}
