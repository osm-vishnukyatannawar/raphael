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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { SaveSettings } from '@wails/go/main/App'
import { settings } from '@wails/go/models'

/** Mirrors settings.MinRefreshSeconds on the Go side. */
const MIN_INTERVAL = 15

/** Values match Go's time.Weekday, which is what the Go side stores. */
const WEEKDAYS = [
  { value: '0', label: 'Sunday' },
  { value: '1', label: 'Monday' },
  { value: '2', label: 'Tuesday' },
  { value: '3', label: 'Wednesday' },
  { value: '4', label: 'Thursday' },
  { value: '5', label: 'Friday' },
  { value: '6', label: 'Saturday' },
]

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
      <DialogContent className="sm:max-w-md">
        {/*
          The form is mounted only while the dialog is open, so its state is
          seeded fresh from `current` on every open. Syncing it with an effect
          instead would be a cascading render (react-hooks/set-state-in-effect).
        */}
        {open && (
          <SettingsForm
            current={current}
            onSaved={onSaved}
            onClose={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function SettingsForm({
  current,
  onSaved,
  onClose,
}: {
  current: settings.Settings
  onSaved: (next: settings.Settings) => void
  onClose: () => void
}) {
  const [tasksInterval, setTasksInterval] = useState(
    String(current.refreshIntervalSeconds)
  )
  const [billingInterval, setBillingInterval] = useState(
    String(current.billingRefreshIntervalSeconds)
  )
  const [myTasksInterval, setMyTasksInterval] = useState(
    String(current.myTasksRefreshIntervalSeconds)
  )
  const [teamInterval, setTeamInterval] = useState(
    String(current.teamRefreshIntervalSeconds)
  )
  const [weekStart, setWeekStart] = useState(String(current.weekStartDay))
  const [notify, setNotify] = useState(current.notifyNewTasks)
  const [focus, setFocus] = useState(current.focusOnNewTask)
  const [notifyMine, setNotifyMine] = useState(current.notifyNewMyTasks)
  const [notifySeconds, setNotifySeconds] = useState(
    String(current.notificationTimeoutSeconds)
  )
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const valid =
    isInterval(tasksInterval) &&
    isInterval(billingInterval) &&
    isInterval(myTasksInterval) &&
    isInterval(teamInterval) &&
    isInterval(notifySeconds)

  async function save(event: React.FormEvent) {
    event.preventDefault()
    if (!valid || busy) return

    setBusy(true)
    setError(null)
    try {
      // Go clamps the values, so the response is the source of truth for what
      // was actually stored — not whatever was typed here.
      const saved = await SaveSettings(
        settings.Settings.createFrom({
          ...current,
          refreshIntervalSeconds: Number(tasksInterval),
          billingRefreshIntervalSeconds: Number(billingInterval),
          myTasksRefreshIntervalSeconds: Number(myTasksInterval),
          teamRefreshIntervalSeconds: Number(teamInterval),
          weekStartDay: Number(weekStart),
          notifyNewTasks: notify,
          focusOnNewTask: focus,
          notifyNewMyTasks: notifyMine,
          notificationTimeoutSeconds: Number(notifySeconds),
        })
      )
      onSaved(saved)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void save(e)} className="flex flex-col gap-5">
      <DialogHeader>
        <DialogTitle>Settings</DialogTitle>
        <DialogDescription>
          How often Raphael checks Pinestem, and how it tells you about new
          work.
        </DialogDescription>
      </DialogHeader>

      <fieldset className="space-y-3">
        <legend className="mb-2 text-sm font-medium">Refresh</legend>

        <IntervalField
          id="tasks-interval"
          label="Tasks in review"
          value={tasksInterval}
          onChange={setTasksInterval}
        />
        <IntervalField
          id="my-tasks-interval"
          label="My tasks"
          value={myTasksInterval}
          onChange={setMyTasksInterval}
        />
        <IntervalField
          id="billing-interval"
          label="Billing hours"
          value={billingInterval}
          onChange={setBillingInterval}
        />
        <IntervalField
          id="team-interval"
          label="Team boards"
          value={teamInterval}
          onChange={setTeamInterval}
        />
        <p className="text-muted-foreground text-xs">
          Seconds. 0 turns automatic refresh off; anything under {MIN_INTERVAL}s
          is raised to {MIN_INTERVAL}s.
        </p>
      </fieldset>

      <fieldset className="space-y-3">
        <legend className="mb-2 text-sm font-medium">New tasks</legend>

        <ToggleField
          id="notify"
          label="Notify me"
          hint="A desktop notification when a task lands in review."
          checked={notify}
          onCheckedChange={setNotify}
        />
        <ToggleField
          id="focus"
          label="Bring Raphael to the front"
          hint="Maximises and raises the window so the new task is visible straight away."
          checked={focus}
          onCheckedChange={setFocus}
        />
        <ToggleField
          id="notify-mine"
          label="Notify me about newly assigned tasks"
          hint="The wider My tasks list. It never pulls the window forward, and hidden tasks stay silent."
          checked={notifyMine}
          onCheckedChange={setNotifyMine}
        />

        {(notify || notifyMine) && (
          <>
            <IntervalField
              id="notify-seconds"
              label="Notification stays for"
              value={notifySeconds}
              onChange={setNotifySeconds}
            />
            <p className="text-muted-foreground text-xs">
              Seconds.{' '}
              <strong>0 keeps it on screen until you dismiss it</strong> —
              unlike the refresh fields above, 0 does not mean off. Windows uses
              its own system toast duration and ignores this.
            </p>
          </>
        )}
      </fieldset>

      <div className="space-y-2">
        <Label htmlFor="week-start">Week starts on</Label>
        <Select value={weekStart} onValueChange={setWeekStart}>
          <SelectTrigger id="week-start" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {WEEKDAYS.map((day) => (
              <SelectItem key={day.value} value={day.value}>
                {day.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-muted-foreground text-xs">
          Sets the window for the “this week” billing total.
        </p>
      </div>

      {error && <p className="text-destructive text-xs">{error}</p>}

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

function IntervalField({
  id,
  label,
  value,
  onChange,
}: {
  id: string
  label: string
  value: string
  onChange: (next: string) => void
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <Label htmlFor={id} className="font-normal">
        {label}
      </Label>
      <div className="flex items-center gap-2">
        <Input
          id={id}
          type="number"
          min={0}
          step={5}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className="w-24"
          aria-invalid={!isInterval(value)}
        />
        <span className="text-muted-foreground w-6 text-sm">s</span>
      </div>
    </div>
  )
}

function ToggleField({
  id,
  label,
  hint,
  checked,
  onCheckedChange,
}: {
  id: string
  label: string
  hint: string
  checked: boolean
  onCheckedChange: (next: boolean) => void
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="space-y-0.5">
        <Label htmlFor={id} className="font-normal">
          {label}
        </Label>
        <p className="text-muted-foreground text-xs">{hint}</p>
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  )
}

function isInterval(value: string): boolean {
  const parsed = Number(value)

  return value.trim() !== '' && Number.isFinite(parsed) && parsed >= 0
}
