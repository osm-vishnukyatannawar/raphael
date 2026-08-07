import TaskRow, { type RowTask } from '@/components/TaskRow'
import { Badge } from '@/components/ui/badge'
import { OpenTask } from '@wails/go/main/App'
import { type team } from '@wails/go/models'

type Props = {
  view: team.BoardView
}

/**
 * One column per person, in the order the board lists them.
 *
 * Grouped rather than flat because the question this answers is "what does each
 * of them have on", which you read person by person. Within a column the Go
 * side has already sorted by due date with undated last, matching My tasks.
 */
export default function TeamTaskBoard({ view }: Props) {
  if (view.groups.length === 0) {
    return <Empty />
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      {view.groups.map((group) => (
        <section key={group.empId} className="min-w-0">
          <h3 className="mb-2 flex items-center gap-2 text-sm font-medium">
            <span className="truncate">{group.empName}</span>
            <Badge variant="secondary" className="shrink-0">
              {group.tasks.length}
            </Badge>
          </h3>

          {group.tasks.length === 0 ? (
            <p className="text-muted-foreground rounded-lg border border-dashed px-4 py-6 text-center text-xs">
              Nothing in these statuses.
            </p>
          ) : (
            <ul className="divide-y rounded-lg border">
              {group.tasks.map((task) => (
                <TaskRow
                  key={task.taskId}
                  task={task}
                  onOpen={open}
                  showStatus
                />
              ))}
            </ul>
          )}
        </section>
      ))}
    </div>
  )
}

function Empty() {
  return (
    <div className="text-muted-foreground rounded-lg border border-dashed py-12 text-center text-sm">
      No people on this board yet.
    </div>
  )
}

/*
  No acknowledgement step, unlike My tasks: a team board never highlights
  anything as new, so there is nothing for opening a task to clear.
*/
function open(task: RowTask) {
  void OpenTask(task.taskId, task.shortCode)
}
