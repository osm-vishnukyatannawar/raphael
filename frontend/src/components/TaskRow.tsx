import { ExternalLink, EyeOff } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import {
  formatDate,
  isOverdue,
  parsePinestemDate,
  relativeTime,
} from '@/lib/time'

/**
 * The fields a row needs, structurally rather than by class.
 *
 * Both `tasks.Task` and `mytasks.Task` satisfy this; naming either one here
 * would force a cast on the other for no gain.
 */
export type RowTask = {
  taskId: number
  shortCode: string
  name: string
  projectName: string
  priority: string
  statusType: string
  statusColor: string
  dueDate: string
  modifiedOn: string
  isNew: boolean
}

type Props = {
  task: RowTask
  onOpen: (task: RowTask) => void
  /** Shown by the assigned list, where rows span every status. */
  showStatus?: boolean
  /** When given, the row grows a hide button. */
  onHide?: (task: RowTask) => void
}

/**
 * One task row, shared by the review queue and the assigned list.
 *
 * The hide button is nested inside the row button in the DOM order sense only —
 * it is a sibling, because a button inside a button is invalid HTML and
 * WebKitGTK and Chromium disagree about what the click then does.
 */
export default function TaskRow({ task, onOpen, showStatus, onHide }: Props) {
  const due = parsePinestemDate(task.dueDate)
  const modified = parsePinestemDate(task.modifiedOn)
  const overdue = isOverdue(due)

  return (
    <li className="relative">
      <button
        type="button"
        onClick={() => onOpen(task)}
        className={`group flex w-full items-start gap-3 border-l-2 py-3 pr-4 pl-4 text-left transition-colors ${
          onHide ? 'pr-20' : ''
        } ${
          task.isNew
            ? 'border-l-primary bg-primary/5 hover:bg-primary/10'
            : 'hover:bg-accent/50 border-l-transparent'
        }`}
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
            {task.isNew && (
              <Badge variant="default" className="shrink-0 text-[10px]">
                New
              </Badge>
            )}
          </div>

          <p className="text-muted-foreground mt-1 text-xs">
            {task.projectName}
            {showStatus && task.statusType && ` · ${task.statusType}`}
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

      {onHide && (
        <button
          type="button"
          title="Hide this task"
          aria-label={`Hide ${task.shortCode}`}
          onClick={() => onHide(task)}
          className="text-muted-foreground hover:bg-accent hover:text-foreground absolute top-2.5 right-9 rounded p-1.5 opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100 [li:hover_&]:opacity-100"
        >
          <EyeOff className="size-3.5" />
        </button>
      )}
    </li>
  )
}
