import { Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'

import MultiSelect, { type Option } from '@/components/MultiSelect'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  DeleteTeamBoard,
  ListSelectableProjects,
  ListTeamMembers,
  ListTeamStatuses,
  SaveTeamBoard,
} from '@wails/go/main/App'
import { type pinestem, team } from '@wails/go/models'

/** Mirrors team.KindTasks / team.KindBilling on the Go side. */
const KIND_TASKS = 'tasks'
const KIND_BILLING = 'billing'

/**
 * Past this many people, a task board's refresh is worth warning about: the
 * Pinestem endpoint has no multi-assignee filter, so each one is a separate
 * paginated request.
 */
const BUSY_MEMBER_COUNT = 10

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** null creates a new board of `kind`. */
  existing: team.Board | null
  /** Only used when creating; an existing board's kind cannot change. */
  kind: string
  onSaved: () => void
}

export default function TeamBoardEditor({
  open,
  onOpenChange,
  existing,
  kind,
  onSaved,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        {/* Mounted only while open so the form seeds fresh from `existing`
            rather than syncing through an effect. */}
        {open && (
          <EditorForm
            existing={existing}
            kind={existing?.kind ?? kind}
            onSaved={onSaved}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function EditorForm({
  existing,
  kind,
  onSaved,
  onClose,
}: {
  existing: team.Board | null
  kind: string
  onSaved: () => void
  onClose: () => void
}) {
  const [name, setName] = useState(existing?.name ?? '')
  const [projects, setProjects] = useState<pinestem.Project[]>([])
  const [selectedProjects, setSelectedProjects] = useState<string[]>(
    existing?.projects.map((p) => String(p.projectId)) ?? []
  )
  const [members, setMembers] = useState<pinestem.Member[]>([])
  const [selectedMembers, setSelectedMembers] = useState<string[]>(
    existing?.members.map((m) => String(m.empId)) ?? []
  )
  const [statuses, setStatuses] = useState<pinestem.TaskStatus[]>([])
  const [selectedStatuses, setSelectedStatuses] = useState<string[]>(
    existing?.statuses.map((s) => String(s.statusId)) ?? []
  )
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const isTasks = kind === KIND_TASKS

  useEffect(() => {
    let cancelled = false

    void (async () => {
      try {
        const list = await ListSelectableProjects()
        if (!cancelled) setProjects(list ?? [])
      } catch (err) {
        if (!cancelled) setError(message(err))
      }
    })()

    return () => {
      cancelled = true
    }
  }, [])

  const chosenProjects = projects.filter((p) =>
    selectedProjects.includes(String(p.projectId))
  )
  const chosenCodes = chosenProjects.map((p) => p.code).join(',')

  // Members follow the chosen projects. With none chosen the Go side answers
  // with the whole company, so the picker is useful before a project is picked
  // and narrows once one is.
  useEffect(() => {
    let cancelled = false
    const codes = chosenCodes ? chosenCodes.split(',') : []

    void (async () => {
      try {
        const list = await ListTeamMembers(codes)
        if (!cancelled) setMembers(list ?? [])
      } catch (err) {
        if (!cancelled) setError(message(err))
      }
    })()

    return () => {
      cancelled = true
    }
  }, [chosenCodes])

  // Statuses are per project and only a task board filters by them.
  useEffect(() => {
    if (!isTasks) return

    let cancelled = false
    const codes = chosenCodes ? chosenCodes.split(',') : []

    void (async () => {
      try {
        const list = await ListTeamStatuses(codes)
        if (!cancelled) setStatuses(list ?? [])
      } catch (err) {
        if (!cancelled) setError(message(err))
      }
    })()

    return () => {
      cancelled = true
    }
  }, [chosenCodes, isTasks])

  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (busy) return

    setBusy(true)
    setError(null)
    try {
      await SaveTeamBoard(
        team.Board.createFrom({
          id: existing?.id ?? 0,
          kind,
          name: name.trim(),
          position: existing?.position ?? 0,
          projects: chosenProjects.map((p) => ({
            projectId: p.projectId,
            code: p.code,
            name: p.name,
          })),
          members: selectedMembers.map((id) => ({
            empId: Number(id),
            name: labelFor(id, knownMembers),
          })),
          statuses: isTasks
            ? selectedStatuses.map((id) => ({
                statusId: Number(id),
                name: labelFor(id, knownStatuses),
              }))
            : [],
        })
      )
      onSaved()
      onClose()
    } catch (err) {
      setError(message(err))
      setBusy(false)
    }
  }

  async function remove() {
    if (!existing) return

    setBusy(true)
    try {
      await DeleteTeamBoard(existing.id)
      onSaved()
      onClose()
    } catch (err) {
      setError(message(err))
      setBusy(false)
    }
  }

  const projectOptions: Option[] = projects.map((p) => ({
    value: String(p.projectId),
    label: p.name,
    hint: p.code,
  }))

  /*
    Options merge what the API currently offers with whatever the board already
    has saved. Narrowing the project selection re-scopes the member and status
    lists, and without this an already-chosen person would drop out of the
    options and be silently deselected on save.
  */
  const knownMembers = merge(
    members.map((m) => [String(m.id), m.name] as const),
    (existing?.members ?? []).map((m) => [String(m.empId), m.name] as const)
  )
  const knownStatuses = merge(
    statuses.map((s) => [String(s.id), s.name] as const),
    (existing?.statuses ?? []).map((s) => [String(s.statusId), s.name] as const)
  )

  const memberOptions: Option[] = [...knownMembers].map(([value, label]) => ({
    value,
    label,
  }))
  const statusOptions: Option[] = [...knownStatuses].map(([value, label]) => ({
    value,
    label,
  }))

  // Matches Board.Configured on the Go side. A board saved without all of these
  // would refresh into nothing, so it is blocked here rather than silently
  // producing an empty board.
  const valid =
    name.trim() !== '' &&
    chosenProjects.length > 0 &&
    selectedMembers.length > 0 &&
    (!isTasks || selectedStatuses.length > 0)

  return (
    <form onSubmit={(e) => void save(e)} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>
          {existing
            ? 'Edit board'
            : isTasks
              ? 'New task board'
              : 'New hours board'}
        </DialogTitle>
        <DialogDescription>
          {isTasks
            ? 'See what a group of people currently have on, across the projects you pick.'
            : 'See what a group of people billed today, yesterday and this week.'}
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="board-name">Name</Label>
        <Input
          id="board-name"
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={isTasks ? 'e.g. Backend squad' : 'e.g. Backend hours'}
        />
      </div>

      <div className="space-y-2">
        <Label>Projects</Label>
        <MultiSelect
          options={projectOptions}
          selected={selectedProjects}
          onChange={setSelectedProjects}
          placeholder="Select projects"
          emptyMessage="No projects found"
        />
      </div>

      <div className="space-y-2">
        <Label>People</Label>
        <MultiSelect
          options={memberOptions}
          selected={selectedMembers}
          onChange={setSelectedMembers}
          placeholder="Select people"
          emptyMessage="No members found"
        />
        {isTasks && selectedMembers.length > BUSY_MEMBER_COUNT && (
          <p className="text-muted-foreground text-xs">
            {selectedMembers.length} people means {selectedMembers.length}{' '}
            separate requests per refresh — Pinestem has no way to ask about
            several at once. Consider a slower team interval in Settings.
          </p>
        )}
      </div>

      {isTasks && (
        <div className="space-y-2">
          <Label>Statuses</Label>
          <MultiSelect
            options={statusOptions}
            selected={selectedStatuses}
            onChange={setSelectedStatuses}
            placeholder={
              chosenProjects.length === 0
                ? 'Pick projects first'
                : 'Select statuses'
            }
            emptyMessage="No statuses on these projects"
            disabled={chosenProjects.length === 0}
          />
        </div>
      )}

      {error && <p className="text-destructive text-xs">{error}</p>}

      <DialogFooter className="gap-2 sm:justify-between">
        {existing ? (
          <Button
            type="button"
            variant="ghost"
            className="text-destructive"
            disabled={busy}
            onClick={() => void remove()}
          >
            <Trash2 className="size-4" />
            Delete
          </Button>
        ) : (
          <span />
        )}

        <div className="flex gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={!valid || busy}>
            {busy ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </DialogFooter>
    </form>
  )
}

export { KIND_BILLING, KIND_TASKS }

/** Live options first, then anything only the saved board still knows about. */
function merge(
  live: readonly (readonly [string, string])[],
  saved: readonly (readonly [string, string])[]
): Map<string, string> {
  const out = new Map(live)
  for (const [value, label] of saved) {
    if (!out.has(value)) out.set(value, label)
  }

  return out
}

function labelFor(id: string, known: Map<string, string>): string {
  return known.get(id) ?? id
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}
