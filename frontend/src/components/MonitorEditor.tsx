import { Trash2 } from 'lucide-react'
import { useCallback, useEffect, useState } from 'react'

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
import { Switch } from '@/components/ui/switch'
import { formatHours } from '@/lib/time'
import {
  DeleteMonitor,
  ListProjectMembers,
  ListSelectableProjects,
  SaveMonitor,
} from '@wails/go/main/App'
import { monitor, type pinestem } from '@wails/go/models'

/** Mirrors monitor.AllProjects on the Go side: "across every project". */
const ALL_PROJECTS = 0

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** null creates a new monitor. */
  existing: monitor.Monitor | null
  onSaved: () => void
}

export default function MonitorEditor({
  open,
  onOpenChange,
  existing,
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
            onSaved={onSaved}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

type MemberDraft = {
  empId: number
  empName: string
  /** Hours as typed, kept as a string so a half-typed value isn't clobbered. */
  totalHours: string
  splitByProject: boolean
  /** projectId → hours, only meaningful when splitByProject. */
  perProject: Record<number, string>
}

function EditorForm({
  existing,
  onSaved,
  onClose,
}: {
  existing: monitor.Monitor | null
  onSaved: () => void
  onClose: () => void
}) {
  const [name, setName] = useState(existing?.name ?? '')
  const [projects, setProjects] = useState<pinestem.Project[]>([])
  const [selectedProjects, setSelectedProjects] = useState<string[]>(
    existing?.projects.map((p) => String(p.projectId)) ?? []
  )
  const [members, setMembers] = useState<pinestem.Member[]>([])
  const [drafts, setDrafts] = useState<MemberDraft[]>(() => toDrafts(existing))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    void (async () => {
      try {
        const list = await ListSelectableProjects()
        if (!cancelled) setProjects(list ?? [])
      } catch (err) {
        if (!cancelled)
          setError(err instanceof Error ? err.message : String(err))
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

  // Members follow the chosen projects: the picker should only offer people who
  // are actually on the work being measured.
  useEffect(() => {
    let cancelled = false
    const codes = chosenCodes ? chosenCodes.split(',') : []

    void (async () => {
      try {
        const list = await ListProjectMembers(codes)
        if (!cancelled) setMembers(list ?? [])
      } catch (err) {
        if (!cancelled)
          setError(err instanceof Error ? err.message : String(err))
      }
    })()

    return () => {
      cancelled = true
    }
  }, [chosenCodes])

  const updateDraft = useCallback(
    (empId: number, patch: Partial<MemberDraft>) => {
      setDrafts((prev) =>
        prev.map((d) => (d.empId === empId ? { ...d, ...patch } : d))
      )
    },
    []
  )

  function addMember(empIds: string[]) {
    setDrafts((prev) => {
      const keep = prev.filter((d) => empIds.includes(String(d.empId)))
      const existingIds = new Set(keep.map((d) => String(d.empId)))

      const added = empIds
        .filter((id) => !existingIds.has(id))
        .map((id) => {
          const found = members.find((m) => String(m.id) === id)

          return {
            empId: Number(id),
            empName: found?.name ?? id,
            totalHours: '',
            splitByProject: false,
            perProject: {},
          }
        })

      return [...keep, ...added]
    })
  }

  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (busy) return

    setBusy(true)
    setError(null)
    try {
      await SaveMonitor(
        monitor.Monitor.createFrom({
          id: existing?.id ?? 0,
          name: name.trim(),
          projects: chosenProjects.map((p) => ({
            projectId: p.projectId,
            code: p.code,
            name: p.name,
          })),
          targets: buildTargets(drafts, chosenProjects),
        })
      )
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  async function remove() {
    if (!existing) return

    setBusy(true)
    try {
      await DeleteMonitor(existing.id)
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  const projectOptions: Option[] = projects.map((p) => ({
    value: String(p.projectId),
    label: p.name,
    hint: p.code,
  }))
  const memberOptions: Option[] = members.map((m) => ({
    value: String(m.id),
    label: m.name,
  }))

  const valid = name.trim() !== '' && chosenProjects.length > 0

  return (
    <form onSubmit={(e) => void save(e)} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>{existing ? 'Edit monitor' : 'New monitor'}</DialogTitle>
        <DialogDescription>
          Track a month's billing target across a group of projects and people.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="monitor-name">Name</Label>
        <Input
          id="monitor-name"
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Acme — Q3 delivery"
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
          selected={drafts.map((d) => String(d.empId))}
          onChange={addMember}
          placeholder={
            chosenProjects.length === 0
              ? 'Pick projects first'
              : 'Select people'
          }
          emptyMessage="No members on these projects"
          disabled={chosenProjects.length === 0}
        />
      </div>

      {drafts.length > 0 && (
        <div className="space-y-3">
          <Label>Monthly targets (hours)</Label>
          {drafts.map((draft) => (
            <div key={draft.empId} className="space-y-2 rounded-md border p-3">
              <div className="flex items-center justify-between gap-3">
                <span className="min-w-0 truncate text-sm font-medium">
                  {draft.empName}
                </span>
                {!draft.splitByProject && (
                  <Input
                    type="number"
                    min={0}
                    step={1}
                    value={draft.totalHours}
                    onChange={(e) =>
                      updateDraft(draft.empId, { totalHours: e.target.value })
                    }
                    className="w-24"
                    placeholder="0"
                  />
                )}
              </div>

              {chosenProjects.length > 1 && (
                <div className="flex items-center justify-between gap-3">
                  <span className="text-muted-foreground text-xs">
                    Split by project
                  </span>
                  <Switch
                    checked={draft.splitByProject}
                    onCheckedChange={(on) =>
                      updateDraft(draft.empId, { splitByProject: on })
                    }
                  />
                </div>
              )}

              {draft.splitByProject && (
                <div className="space-y-2 border-t pt-2">
                  {chosenProjects.map((p) => (
                    <div
                      key={p.projectId}
                      className="flex items-center justify-between gap-3"
                    >
                      <span className="text-muted-foreground min-w-0 truncate text-xs">
                        {p.name}
                      </span>
                      <Input
                        type="number"
                        min={0}
                        step={1}
                        value={draft.perProject[p.projectId] ?? ''}
                        onChange={(e) =>
                          updateDraft(draft.empId, {
                            perProject: {
                              ...draft.perProject,
                              [p.projectId]: e.target.value,
                            },
                          })
                        }
                        className="w-24"
                        placeholder="0"
                      />
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}

          {/*
            Read back through the shared formatter, so the total below the
            inputs is in the same h:mm the rest of the app shows. The inputs
            themselves stay decimal hours — typing 7.5 is easier than 7:30 on a
            number field, and this line is what confirms it landed as 7:30.
          */}
          <p className="text-muted-foreground text-xs">
            Total: {formatHours(totalTargetMinutes(drafts))} per month
          </p>
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

function toDrafts(existing: monitor.Monitor | null): MemberDraft[] {
  if (!existing) return []

  const byEmp = new Map<number, MemberDraft>()

  for (const t of existing.targets) {
    const draft = byEmp.get(t.empId) ?? {
      empId: t.empId,
      empName: t.empName,
      totalHours: '',
      splitByProject: false,
      perProject: {},
    }

    if (t.projectId === ALL_PROJECTS) {
      draft.totalHours = String(t.targetMinutes / 60)
    } else {
      draft.splitByProject = true
      draft.perProject[t.projectId] = String(t.targetMinutes / 60)
    }

    byEmp.set(t.empId, draft)
  }

  return [...byEmp.values()]
}

/**
 * Turns the drafts into target rows.
 *
 * A split member emits one row per project; an unsplit one emits a single row
 * against the ALL_PROJECTS sentinel. Rows with no hours are dropped rather than
 * stored as zero, so an untouched person doesn't count as a 0h commitment.
 */
function buildTargets(
  drafts: MemberDraft[],
  chosenProjects: pinestem.Project[]
): monitor.Target[] {
  const targets: monitor.Target[] = []

  for (const draft of drafts) {
    if (draft.splitByProject) {
      for (const p of chosenProjects) {
        const minutes = toMinutes(draft.perProject[p.projectId])
        if (minutes > 0) {
          targets.push(
            monitor.Target.createFrom({
              empId: draft.empId,
              empName: draft.empName,
              projectId: p.projectId,
              targetMinutes: minutes,
            })
          )
        }
      }

      continue
    }

    const minutes = toMinutes(draft.totalHours)
    if (minutes > 0) {
      targets.push(
        monitor.Target.createFrom({
          empId: draft.empId,
          empName: draft.empName,
          projectId: ALL_PROJECTS,
          targetMinutes: minutes,
        })
      )
    }
  }

  return targets
}

function toMinutes(hours: string | undefined): number {
  const parsed = Number(hours)
  if (!Number.isFinite(parsed) || parsed <= 0) return 0

  return Math.round(parsed * 60)
}

function totalTargetMinutes(drafts: MemberDraft[]): number {
  let minutes = 0

  for (const draft of drafts) {
    if (draft.splitByProject) {
      for (const value of Object.values(draft.perProject)) {
        minutes += toMinutes(value)
      }
    } else {
      minutes += toMinutes(draft.totalHours)
    }
  }

  return minutes
}
