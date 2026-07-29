import { useState } from 'react'

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
import { SaveSettings } from '@wails/go/main/App'
import { type settings } from '@wails/go/models'

/** Mirrors settings.MinRefreshSeconds on the Go side. */
const MIN_INTERVAL = 15

type Props = {
  open: boolean
  onOpenChange: (open: boolean) => void
  current: settings.Settings
  onSaved: (next: settings.Settings) => void
}

export default function SettingsDialog({
  open,
  onOpenChange,
  current,
  onSaved,
}: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        {/*
          The form is mounted only while the dialog is open, so its state is
          seeded fresh from `current` on every open. Syncing it with an effect
          instead would be a cascading render (react-hooks/set-state-in-effect).
        */}
        {open && (
          <IntervalForm
            current={current}
            onSaved={onSaved}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function IntervalForm({
  current,
  onSaved,
  onClose,
}: {
  current: settings.Settings
  onSaved: (next: settings.Settings) => void
  onClose: () => void
}) {
  const [value, setValue] = useState(String(current.refreshIntervalSeconds))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const parsed = Number(value)
  const valid = value.trim() !== '' && Number.isFinite(parsed) && parsed >= 0

  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (!valid || busy) return

    setBusy(true)
    setError(null)
    try {
      // Go clamps the value, so the response is the source of truth for what
      // was actually stored — not whatever was typed here.
      const saved = await SaveSettings(parsed)
      onSaved(saved)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void save(e)} className="flex flex-col gap-4">
      <DialogHeader>
        <DialogTitle>Settings</DialogTitle>
        <DialogDescription>
          How often Raphael checks Pinestem for changes.
        </DialogDescription>
      </DialogHeader>

      <div className="space-y-2">
        <Label htmlFor="interval">Auto-refresh every</Label>
        <div className="flex items-center gap-2">
          <Input
            id="interval"
            type="number"
            min={0}
            step={5}
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="w-28"
          />
          <span className="text-muted-foreground text-sm">seconds</span>
        </div>
        <p className="text-muted-foreground text-xs">
          0 turns automatic refresh off. Anything under {MIN_INTERVAL}s is
          raised to {MIN_INTERVAL}s.
        </p>
        {error && <p className="text-destructive text-xs">{error}</p>}
      </div>

      <DialogFooter>
        <Button type="button" variant="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" disabled={!valid || busy}>
          {busy ? 'Saving…' : 'Save'}
        </Button>
      </DialogFooter>
    </form>
  )
}
