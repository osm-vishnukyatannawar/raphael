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
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  GetMyTasksFilter,
  GetMyTasksOptions,
  SaveMyTasksFilter,
} from '@wails/go/main/App'
import { mytasks } from '@wails/go/models'

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}

export default function MyTasksFilterDialog({
  open,
  onOpenChange,
  onSaved,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        {/*
          Mounted only while open, so the form state and the option lists are
          fetched fresh each time. Syncing them with an effect instead would be
          a cascading render (react-hooks/set-state-in-effect).
        */}
        {open && (
          <FilterForm onSaved={onSaved} onClose={() => onOpenChange(false)} />
        )}
      </DialogContent>
    </Dialog>
  )
}

function FilterForm({
  onSaved,
  onClose,
}: {
  onSaved: () => void
  onClose: () => void
}) {
  const [options, setOptions] = useState<mytasks.Options | null>(null)
  const [projects, setProjects] = useState<string[]>([])
  const [statuses, setStatuses] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Options come from the API rather than the cache: a status list that has
  // drifted is exactly how a saved filter silently stops matching anything.
  useEffect(() => {
    let cancelled = false

    void (async () => {
      try {
        const [loaded, saved] = await Promise.all([
          GetMyTasksOptions(),
          GetMyTasksFilter(),
        ])
        if (cancelled) return

        setOptions(loaded)
        setProjects(saved.projects.map((p) => p.code))
        setStatuses(saved.statuses.map((s) => String(s.id)))
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : String(err))
        }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [])

  const projectOptions: Option[] = (options?.projects ?? []).map((p) => ({
    value: p.code,
    label: p.name,
    hint: p.code,
  }))

  const statusOptions: Option[] = (options?.statuses ?? []).map((s) => ({
    value: String(s.id),
    label: s.name,
    hint: s.isDone ? 'done' : undefined,
  }))

  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (busy || !options) return

    setBusy(true)
    setError(null)
    try {
      await SaveMyTasksFilter(
        mytasks.Filter.createFrom({
          projects: options.projects
            .filter((p) => projects.includes(p.code))
            .map((p) => ({ code: p.code, name: p.name })),
          statuses: options.statuses
            .filter((s) => statuses.includes(String(s.id)))
            .map((s) => ({ id: s.id, name: s.name })),
        })
      )
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void save(e)} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>Filter my tasks</DialogTitle>
        <DialogDescription>
          Which projects and statuses the assigned list covers. Leave either one
          empty for no filter — every active project, and every status except
          done.
        </DialogDescription>
      </DialogHeader>

      {options === null && !error ? (
        <div className="space-y-3">
          <Skeleton className="h-9 w-full" />
          <Skeleton className="h-9 w-full" />
        </div>
      ) : (
        <>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Projects</Label>
              {projects.length > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setProjects([])}
                >
                  Clear
                </Button>
              )}
            </div>
            <MultiSelect
              options={projectOptions}
              selected={projects}
              onChange={setProjects}
              placeholder="All active projects"
              emptyMessage="No projects found."
            />
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label>Statuses</Label>
              {statuses.length > 0 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setStatuses([])}
                >
                  Clear
                </Button>
              )}
            </div>
            <MultiSelect
              options={statusOptions}
              selected={statuses}
              onChange={setStatuses}
              placeholder="Every status except done"
              emptyMessage="No statuses found."
            />
          </div>
        </>
      )}

      {error && <p className="text-destructive text-xs">{error}</p>}

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={busy || options === null}>
          {busy ? 'Saving…' : 'Save'}
        </Button>
      </DialogFooter>
    </form>
  )
}
