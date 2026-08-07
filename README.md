# Raphael

Cross-platform desktop app (Linux + Windows) built with [Wails v2](https://wails.io):
a Go backend bound to a React + TypeScript frontend running in the OS webview.

It signs in to Pinestem, shows the tasks waiting on your review alongside
everything else assigned to you, alerts you when a new one arrives, tracks the
hours you've billed today and this week, and monitors monthly billing targets for
a team across a group of projects.

## Install (Linux)

One line, no sudo — the same command installs and updates:

```sh
wget -qO- https://raw.githubusercontent.com/osm-vishnukyatannawar/raphael/main/install.sh | bash
```

or with curl:

```sh
curl -fsSL https://raw.githubusercontent.com/osm-vishnukyatannawar/raphael/main/install.sh | bash
```

It resolves the latest release, verifies it against the published
`SHA256SUMS.txt`, and installs the binary into `~/.local/bin` with the icon and
`Raphael.desktop`. Re-running it updates in place and exits early when you are
already on the latest version — `raphael --version` is what it compares against.
`RAPHAEL_VERSION=v1.0.0` pins a specific tag; `RAPHAEL_FORCE=1` reinstalls anyway.

WebKitGTK is the one dependency it cannot ship (see below); the script warns if
it is missing rather than failing after the fact. Windows builds are on the
[releases page](https://github.com/osm-vishnukyatannawar/raphael/releases).

The rest of this file is about working on Raphael, not running it.

## Requirements

| | Version |
|---|---|
| Go | 1.26.4+ (see the `go` directive in `go.mod` — CI reads it directly) |
| Node | 22+ |
| pnpm | 11+ |
| Wails CLI | v2.13.0 — `go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0` |

Linux also needs the WebKitGTK dev headers:

```sh
# Arch
sudo pacman -S webkit2gtk-4.1 gtk3
# Debian/Ubuntu
sudo apt install libwebkit2gtk-4.1-dev libgtk-3-dev
```

Run `wails doctor` to confirm the environment.

> **`wails doctor` reports `libwebkit — Not Found` on Arch-family distros.** That
> is a false negative: it looks for a package named `webkit2gtk` (the 4.0 API),
> while current distros ship `webkit2gtk-4.1`. The Makefile passes
> `-tags webkit2_41` on Linux, and the app builds and runs. Don't chase it.

## Getting started

```sh
make setup   # install tools, frontend deps, and git hooks
make dev     # hot-reloading app window
```

## Commands

| Command | What it does |
|---|---|
| `make dev` | Run with hot reload |
| `make build` | Production binary into `build/bin/` |
| `make lint` | golangci-lint + ESLint + Prettier check |
| `make fmt` | Format Go and frontend in place |
| `make test` | Go tests |
| `make typecheck` | Frontend `tsc -b` |
| `make generate` | Regenerate sqlc queries and Wails bindings |

## Layout

```
main.go, app.go          Wails wiring. Exported App methods become TS bindings.
install.sh               Network installer/updater — the README one-liner runs this
internal/config/         Resolves ~/.config/raphael (%AppData%\raphael on Windows)
internal/db/             SQLite connection + goose migrations (embedded)
  migrations/            Schema, applied on startup
  queries/               sqlc input
  sqlc/                  sqlc output — generated, do not edit
internal/pinestem/       Pinestem REST client (auth, tasks, billing)
internal/identity/       Onboarding state: display name + Pinestem session
internal/secret/         OS keyring access
internal/tasks/          In-review queue: cache, new-task detection, alert wording
internal/mytasks/        Everything assigned to you: filters, hiding, its own alert
internal/alerttext/      Shared notification-body formatting
internal/billing/        Logged hours: per-day cache, week arithmetic
internal/monitor/        Billing targets: monthly progress, pace, working days
internal/team/           Team boards: other people's tasks and hours, per board
internal/settings/       Preferences (intervals, week start, alert toggles)
internal/poller/         The four refresh loops — see "Refresh loops" below
internal/notify/         Desktop notifications + raising the window
internal/ai/             AI integration seam (empty)
frontend/src/            React app
  components/ui/         shadcn components (vendored, owned by us)
frontend/wailsjs/        Generated Go bindings — committed, see note below
```

`frontend/wailsjs/` is generated but **committed**: `tsc` and ESLint need it to
resolve imports, so gitignoring it breaks typecheck and lint on a fresh clone
before anyone has run `wails dev`. Treat churn there as noise, not review surface.

## Stack notes

- **SQLite driver is `modernc.org/sqlite`** (pure Go, no cgo). Wails already
  requires cgo for the webview; keeping the database layer cgo-free avoids adding
  a second cross-compilation constraint.
- **Tailwind v4** runs as a Vite plugin. There is no `tailwind.config.js` and no
  PostCSS config — the theme lives in `frontend/src/index.css` under `@theme`.
  Most tutorials still describe the v3 setup.
- **Path aliases** (`@` → `src`, `@wails` → `wailsjs`) are declared in *both*
  `vite.config.ts` and `tsconfig.app.json`. shadcn reads both; they must agree.
- **Rendering differs per platform.** Linux uses WebKitGTK, Windows uses WebView2
  (Chromium). They are not the same engine. CI builds both on every push so a
  split surfaces on the commit that caused it.

## Identity & Pinestem

First launch onboards in two steps — Pinestem login, then the name Raphael should
call you by (prefilled from the Pinestem account). Later launches load the session
from SQLite and go straight in.

**Where secrets live.** The Pinestem token *and* password go to the OS keyring
(`raphael` service, keys `pinestem-token` / `pinestem-password`). SQLite stores only
non-secret profile data — there is no password column in the schema. If no keyring
is available the app degrades: the token falls back to SQLite's `token_fallback`
column, the password is not stored, and the UI says so.

Inspect what's stored:

```sh
sqlite3 ~/.config/raphael/raphael.db 'SELECT * FROM pinestem_account;'
# Presence only. Do NOT use `secret-tool search --all` here — it prints the
# secret values themselves straight to your terminal and scrollback.
secret-tool lookup service raphael key pinestem-token >/dev/null && echo "token stored"
```

**Detecting a failed login is not obvious.** Pinestem returns HTTP **200** for a
rejected login, with `Status: false` (identical to success) and an *empty*
`ErrorMessage`. The only reliable signal is whether `MultipleResults[0].TokenId` is
present — `internal/pinestem` keys off that and nothing else. Both payload shapes are
pinned as tests in `internal/pinestem/client_test.go`.

Authenticated calls need two headers, `AuthenticationToken` and `CompanyID`, which is
why both are persisted. Use `Client.NewAuthenticatedRequest` rather than setting them
by hand.

## Refresh loops

The refresh loops are **Go goroutines** (`internal/poller`), not `setInterval` in
the frontend. WebKitGTK and Chromium both throttle timers in a hidden page —
Chromium down to one wake per minute after five minutes — and the new-task alert
exists precisely to reach you when the window *isn't* in front. The backend emits
`tasks:updated`, `mytasks:updated`, `billing:updated` and `team:updated`; the
frontend subscribes.

There are four loops with separate intervals — tasks (60s), my tasks (300s),
billing (300s) and team boards (600s) — each with its own on-demand refresh
button. `0` disables any of them; anything under 15s is raised to 15s. The target
monitors ride the billing loop and emit `monitors:updated`.

The team loop is the slowest by default for a reason worth knowing before turning
it down: a task board costs **one paginated request per person on it**, so 15s on
a ten-person board is forty requests a minute.

## Tasks in review

The left-hand column of the dashboard lists the tasks assigned to you that are in
"In review", newest-modified first. Rows open the task in your browser. The window
starts maximised, because the dashboard is two columns wide.

Two calls per refresh, in order:

1. `GET Projects/ProjectsDropdown` → every project you can read
2. `GET Tasks/Filter` with one repeated `ProjectCode` per project

Projects are fetched live rather than hardcoded because they get added and removed.

### New-task alerts

A task that wasn't in the queue on the previous refresh triggers a desktop
notification and pulls the window forward, each toggleable in settings. The row stays
highlighted until you open it or press "Mark all seen".

#### Notifications talk to D-Bus directly on Linux

**Wails' `runtime.SendNotification` hardcodes the `Notify` `expire_timeout` argument
to `-1`** ("server default"), so notification duration was not configurable through
it — the value was never ours to set. `internal/notify/dbus_linux.go` therefore calls
the daemon itself via godbus (already in the dependency graph through Wails and
go-keyring, so no new module). That also buys the `desktop-entry` hint and
`replaces_id`, so repeat alerts update in place instead of stacking.

Duration is a setting: seconds, or **`0` for "until dismissed"**. Note `0` means the
opposite of what it means for the refresh intervals in the same dialog — it is
freedesktop's own convention for `expire_timeout`, passed straight through. Verified
against Plasma 6.7.3: `5000` closes with reason 1 (expired), `0` never closes on its
own.

Windows keeps the Wails path (`wails_other.go`) and ignores the duration — the toast
API only offers "short"/"long" presets, not a duration.

#### Getting the window to actually come forward

`Raise` maximises and presents the window. The call order is load-bearing:
`WindowShow` is `gtk_widget_show`, which only *maps* the window and neither raises nor
focuses it. The one that does both is `WindowUnminimise` — `gtk_window_present`
underneath — so it must come last, after the window is mapped and maximised.

`WindowSetAlwaysOnTop` is `gtk_window_set_keep_above`, an X11 window-manager hint that
**Wayland ignores outright**. The brief pulse helps on X11 and XWayland and does
nothing on a native Wayland session.

**On Wayland, an app cannot focus itself on demand.** Activation needs an
xdg-activation token, which is only issued off the back of user interaction, and a
background alert has none — the compositor decides, not the app. Maximise and raise
still work; KDE typically marks the window as demanding attention rather than
stealing focus. Two reliable routes:

1. **Click the notification.** That *is* user interaction, so it activates the window.
   Requires the desktop entry (below) so KDE can connect the notification to the app.
2. **A KWin rule**, for unconditional auto-raise: System Settings → Window Management
   → Window Rules → New, match *Window class* `Raphael`, then set **Focus stealing
   prevention: None** and **Focus protection: None**.

### Install the desktop entry (Linux)

```sh
make install-desktop   # binary → ~/.local/bin, icon + Raphael.desktop
```

The filename `Raphael.desktop` must match the GTK program name set in `main.go`, which
becomes the Wayland `app_id`. Without it KDE shows a generic window icon and cannot
activate the window from a notification — the same root cause behind both symptoms.

"Which tasks are new" is `task` minus `seen_task`, so it survives restarts.
`seen_task` is rebuilt on every refresh alongside the task cache, which means a task
that leaves review and later returns alerts again — deliberate, it needs another look.

**Migration 00004 seeds `seen_task` from whatever is already cached.** Without that,
upgrading an install with an existing task cache makes every open review look new at
once: a notification per task and the window yanked forward on first launch. Verified
against a real pre-upgrade database, not assumed.

```sh
sqlite3 ~/.config/raphael/raphael.db \
  'SELECT t.short_code, s.acknowledged FROM task t JOIN seen_task s USING (task_id);'
```

## My tasks

The right-hand column of the dashboard is everything assigned to you, not just
what is waiting on your review — the two lists sit side by side because the point
of the second one is to be visible at the same time as the first.

**Ordered by due date, soonest first, undated last.** That is deliberately the
opposite of the review queue's newest-modified order: this list is a backlog, and
what is nearly due matters more than what someone touched five minutes ago.
Overdue dates render in red, the same as everywhere else.

Same endpoint as the review queue, `Tasks/Filter`, with `TaskStatusID` repeated
once per status rather than pinned to `4063`. `ListReviewTasks` and
`ListAssignedTasks` are both one-line wrappers over `ListTasksAssignedTo`.

#### `Tasks/Filter` landmines

Three, all found the hard way and all load-bearing for the team boards:

- **`AssignedTo` takes exactly one employee.** Repeating it
  (`AssignedTo=A&AssignedTo=B`) returns `RecordCount: 0`. Comma-joining it
  (`AssignedTo=A,B`) returns rows whose `AssignedToEmpID` is `0` — plausible-
  looking wrong data, not an error. There is no multi-assignee form; N people
  means N paginated calls, which is the whole cost model of a task board.
- **`ExcludeInformTo=false` returns tasks the person is only *informed* on.**
  Asking about one member with it off came back with rows owned by two other
  people. Your own lists want that (a task you are informed on is still yours to
  see); a team board does not, or a person's column fills with other people's
  work. `ListTasksAssignedTo` takes the flag; the personal lists pass `false`,
  the team boards pass `true`.
- **Rows carry `AssignedToEmpID`**, which is the same identifier as a members-
  dropdown `ID` and a billing row's `EmpID`. Team boards group by it rather than
  by the person they asked about, so a stray row can be dropped rather than
  mis-attributed.

`Projects/ProjectMembersDropdown` with **no** `ProjectCode` returns the whole
company (85 members for 453) rather than erroring. `ListProjectMembers` still
returns empty for an empty code list — an accidentally-empty filter must not
quietly widen to everyone — so asking for the roster goes through the separate
`ListCompanyMembers`.

### Filtering

The **Filter** button picks projects and statuses. Both default to empty, which
means *no* filter: every active project, and every status that is not the terminal
"done" one. That default is what makes the list useful before anyone configures
anything.

Every picker in the app is the same `MultiSelect`, and each one has **Select
all** / **Clear** in its footer — 80 projects or 85 people is not something to
tick one at a time. With a filter typed the buttons narrow to "Select N
matching" / "Clear matching" and act only on what is visible, unioned with
whatever was already chosen, so bulk-picking a subset never discards selections
made under a different filter.

Statuses come from `Projects/ProjectTaskStatuses`, which answers *per project* —
a status shared by ten projects comes back ten times — so the rows are
deduplicated by ID. Company 453 has 15 statuses, of which exactly one
(`1823`, "9. Done") reports `IsDone`. Note the unfiltered list therefore still
includes `2178` ("8. Stopped"); Pinestem's own web UI drops that one too, but
nothing in the payload marks it as terminal, so narrowing it is a filter choice
rather than something to hardcode.

A configured filter costs **one** API call per refresh. An empty one costs three:
the project list, the status list for those projects, then the tasks.

### Hiding

Any row can be hidden, which is how the perpetual tasks — a standing "Project
Management" or "Scrum call" that is assigned to you forever — stay out of the way.

Hiding is per task ID and **outlives the refresh cycle**: `hidden_my_task` is the
one my-task table that is *not* swapped wholesale, because a task that briefly
drops out of the filtered set must not come back unhidden. A hidden task never
counts as new and never triggers a notification, even the first time it appears —
that is what hiding means.

The hide list stores the short code and name it had when hidden, so **"Show
hidden" can still name and unhide a task the current filter no longer returns**.
Without that, narrowing the filter would strand a hidden task with no way back.

```sh
sqlite3 ~/.config/raphael/raphael.db 'SELECT * FROM hidden_my_task;'
```

### Its own cadence and its own alert

The refresh interval is separate from the review queue's (300s by default against
60s) and set separately in the dialog — everything assigned to you turns over far
more slowly than the subset actively waiting on you.

New arrivals notify with their own wording ("assigned to you" rather than "for
review") and their own `replaces_id`, so an assignment alert and a review alert
do not overwrite each other. There is deliberately **no focus-the-window toggle
here**: a task landing in your backlog is worth telling you about, not worth
pulling the window over whatever you were doing.

## Billing hours

Today and this week sit above the task list, with a seven-day breakdown and a lookup
for any single date behind the chevron.

**One call per refresh:** `POST Reports/FilterBillingDetails_New?isGetAllProjects=true`.
It returns per-entry rows, and `internal/billing` aggregates them per day. Summing
them reproduces `Reports/GetBillingTotalHours` exactly (verified: 2160 minutes for
2026-07-27…30), so the totals endpoint is not called at all.

Three things that are easy to get wrong here, all confirmed live:

- **`EmpID` is the user filter, and it is mandatory in practice.** Omit it and the
  endpoint returns *the whole team* — one project for one week came back as 56.08h
  rather than the caller's own hours. `UserIds`, `MemberIds`, `UserID`, `AssignedTo`,
  `EmployeeIds` and four other plausible names are all silently ignored, returning the
  unfiltered figure with no error. The name came from reading Pinestem's own Angular
  bundle. `EmpID` is a parameter rather than "the logged-in user" so reporting on a
  colleague later needs no signature change.
- **`BillableHours` and `NonBillableHours` are integer minutes**, despite the names —
  `300` renders as `"5"` in `BillableHours_HoursFormat`. They stay minutes all the way
  to the UI; 0.1h has no exact binary float representation. Every duration is
  rendered as `h:mm` by `formatHours` in `frontend/src/lib/time.ts` — the one
  formatter, so `12:30` never appears as `12.50 h` somewhere else.
- **Per-row `TotalHours` is always `0`.** The row total is billable + non-billable.

`ProjectIds` is optional; omitting it covers every project, so billing needs no
project list. `Reports/FilterDailyBillingDetails` appears in the bundle but 404s, and
`FilterBillingDetails_New` rejects GET.

**`PageLimit` is capped at 100 server-side.** Asking for 50 returns 50, but 100, 200
and 500 all return exactly 100. `billingPageLimit` must therefore *equal* the cap:
set it higher and a full 100-row page reads as "shorter than the limit, so this is
the last one", every later page is dropped, and the total is quietly wrong rather
than an error. A month across two projects is 434 entries — five pages — so this is
not a theoretical concern. The regression test models the cap rather than honouring
whatever the client asks for, which is exactly how it slipped through the first time.

The fetch window is the configured week stretched back to include yesterday — on the
first day of the week, yesterday is in the *previous* one, and without that the
"Yesterday" figure would silently read zero. Week start is configurable (default
Monday, ISO-8601).

**Day boundaries are computed in the OS local zone, not the account's.** Pinestem
stores `TimeZone` as a *Windows* identifier (`"India Standard Time"`), which Go's
`time.LoadLocation` cannot resolve — it wants IANA names like `Asia/Kolkata`. The
string is still forwarded in the request body, where the Windows form is what the API
expects.

```sh
sqlite3 ~/.config/raphael/raphael.db 'SELECT * FROM billing_day ORDER BY day;'
```

**`AssignedTo` is per-company.** It takes the `UserId` from the auth response, which
differs for the same person in each company — `vishnu.k@osmosys.co` is 1187 at
Amphenol, 2286 at Osmosys, 4928 at TalonPro. `pinestem_account.user_id` stores the
right one, so it must never be swapped for an employee ID from elsewhere.

**`TaskStatusID = 4063`** ("3. In review") is hardcoded in `internal/pinestem/tasks.go`
for the review queue only. It is the one status that list is *about*, and looking it
up would mean matching on a display string each project is free to rename, so this
stays the single place to change if a company numbers statuses differently.

The lookup does exist, though: `Projects/ProjectTaskStatuses?ProjectCode=…&Status=1`,
found later than the four 404ing paths above (`Tasks/TaskStatusDropdown`,
`Masters/TaskStatus`, `Tasks/StatusDropdown`, `Masters/TaskStatusDropdown`). The
my-tasks filter reads the real set from it — see "My tasks" above.

Tasks and projects are cached in SQLite and **replaced wholesale** on each refresh
inside one transaction: a task leaving the queue must disappear, and a mid-refresh
failure leaves the previous cache intact rather than blanking the list.

```sh
sqlite3 ~/.config/raphael/raphael.db \
  'SELECT short_code, project_code, modified_on FROM task ORDER BY modified_on DESC;'
```

## Team boards

The **Team** tab holds any number of named boards over what *other* people are
doing. Two kinds, created and named independently:

- A **task board** — projects + statuses + people — shows what each person
  currently has on, one column per person.
- An **hours board** — projects + people — shows what each person billed today,
  yesterday and this week, with the daily breakdown alongside.

Boards are yours to name and arrange; the strip inside the tab switches between
them. Every board needs projects and people (and statuses, for a task board)
before it fetches anything — see the guard below.

**Team boards never notify.** They report on somebody else's queue, which is not
worth interrupting you for, so there is no first-seen tracking, no "new" badge
and no acknowledgement — the whole `seen_*` machinery that `my_task` carries is
absent from `internal/team` on purpose. One interval covers every board (600s by
default, in the same Settings dialog as the others).

### The two kinds cost wildly different amounts, and that shapes everything

**A task board is one paginated request per person.** `Tasks/Filter` has no
working multi-assignee filter — see the API notes below — so a five-person board
is five requests, run four at a time (`memberFetchLimit`). This is why the team
interval defaults to the slowest of the four, why the editor warns past ten
people, and why every filter is mandatory.

**An hours board is one request, no matter how many boards there are.** Omitting
`EmpID` from the billing endpoint returns the whole team, and every row carries
`EmpID` and `ProjectID`, so one call covers the union of every hours board's
projects and the rows are sliced per board locally. This is the same trick
`internal/monitor` uses. Adding a board is free; adding a *project* is not.

Because that one call spans every board's projects, rows are bucketed by
`(EmpID, ProjectID, day)` and filtered per board before being cached. Bucketing
by person alone would credit a board scoped to one project with hours its members
logged on a different board's project.

### Why every filter is mandatory

Pinestem reads an omitted `ProjectCode` as "every project" and an omitted
`TaskStatusID` as "every status". An under-configured task board would therefore
pull each member's entire history — one member with no filters is 555 tasks
across 6 pages — multiplied by the number of members. So a board with no
projects, no people, or (for a task board) no statuses fetches nothing at all,
reports `configured: false`, and the UI says so rather than showing a
mysteriously empty board. The editor blocks saving one too.

### Storage

`team_board` plus `team_board_project`, `team_board_member` and
`team_board_status` for the configuration, and `team_board_task` /
`team_board_day` for the caches, both replaced wholesale per board per refresh.
Project codes and member names are denormalised onto the board rows for the same
reason `monitor_project` does it: the `project` table is a cache wiped on every
task refresh, so joining would make a saved board's labels depend on whether the
last sync happened to succeed.

```sh
sqlite3 ~/.config/raphael/raphael.db \
  'SELECT b.name, b.kind, COUNT(m.emp_id) FROM team_board b
     LEFT JOIN team_board_member m ON m.board_id = b.id GROUP BY b.id;'
```

## Targets (monitors)

A *monitor* is a saved group — one or more projects, one or more people, each with a
monthly target — answering "are we collectively on track for what this client needs
this month?" rather than just "what have I billed?". It lives on the **Targets** tab.

Targets are per person across the monitor's projects, or split per person per
project. Both are the same model: `monitor_target.project_id = 0` is a sentinel
meaning "across every project in this monitor". It is `0` rather than `NULL` so it can
sit in the primary key — NULLs are distinct from one another in SQLite, which would
let the same person be inserted twice.

**One API call serves every monitor.** The fetch is over the *union* of all monitored
projects, with `EmpID` omitted so it covers the whole team, and the rows are sliced
per monitor in Go using their `EmpID` and `ProjectID`. Adding a monitor therefore
costs no extra requests; only widening the set of projects does. Two monitors may
watch the same project, and an entry counts toward both — the attribution is not a
partition.

**Measured against billable hours**, with non-billable shown alongside but not
counted. Period is the calendar month. Working days are Mon–Fri, and the catch-up
figure **excludes today** so it never assumes a shortfall can still be absorbed in
what is left of the evening. When no working days remain the per-day figure is `0`
and the UI says so, rather than dividing by zero or implying it is achievable.

Each monitor also reports *expected by now* — the target prorated by elapsed working
days — and flags on-track or behind against it. A bare "120 of 200h" means nothing on
the 3rd of the month; the marker on the progress bar is what makes it readable.

The project picker uses statuses 1–5 (80 projects), not the 1–2 the review queue
filters to (37): a project can still accrue hours after it leaves the active
statuses. `monitor_project` denormalises the code and name because the `project`
table is a cache wiped on every task refresh.

Monitors ride the billing poller — same source, same cadence, separate query and
separate `monitors:updated` event.

```sh
sqlite3 ~/.config/raphael/raphael.db \
  'SELECT m.name, t.emp_name, t.project_id, t.target_minutes
     FROM monitor m JOIN monitor_target t ON t.monitor_id = m.id;'
```

> Deleting a monitor cascades to its projects, targets and actuals — but only with
> foreign keys enabled. The app's DSN sets `_pragma=foreign_keys(1)`; the `sqlite3`
> CLI does **not** enable them by default, so a manual `DELETE FROM monitor` there
> leaves orphans behind. Use `PRAGMA foreign_keys=ON;` first if you are poking at the
> database by hand.

## Icon

`build/appicon.svg` is the source of truth; `make icon` rasterizes it to
`build/appicon.png` and `build/windows/icon.ico`.

> On **KDE under Wayland** the window icon shows as a generic placeholder. That is not
> a build problem — KWin ignores the GTK window icon and resolves `app_id` against an
> installed `.desktop` file, which a dev build doesn't have. Verified working under
> `GDK_BACKEND=x11` and in the packaged Windows build.

## Releases

Tag-driven. Pushing a `v*` tag runs `.github/workflows/release.yml`, which re-runs
the tests, builds both platforms on their own runners, and publishes a GitHub release
with the archives and a `SHA256SUMS.txt`.

```sh
git tag -a v1.2.3 -m "Raphael v1.2.3"
git push origin v1.2.3
```

The binaries are **not** cross-compiled. Wails needs cgo for the webview, so Windows
is built on a Windows runner and Linux on a Linux one with the WebKitGTK headers.

The version shown in the app comes from `-ldflags "-X main.version=<tag>"`, so a
build made any other way reports `dev`.

## Commits

This repo follows [Conventional Commits 1.0.0](https://www.conventionalcommits.org/en/v1.0.0/),
enforced by commitlint via a `commit-msg` hook.

```
<type>(<scope>): <subject>
```

**Types:** `feat`, `fix`, `refactor`, `perf`, `docs`, `test`, `build`, `ci`, `chore`, `revert`

**Scopes:** `db`, `api`, `sync`, `ui`, `ai`, `config`, `build`, `ci`, `deps`, `repo`

```sh
feat(db): add accounts table and migration
fix(sync): stop leaking the ticker on shutdown
chore(deps): bump wails to v2.13.0
```

Breaking changes take a `!` before the colon (`feat(api)!: drop v1 endpoints`) or a
`BREAKING CHANGE:` footer.

## Hooks

`make setup` installs Lefthook. On commit it formats and lints staged files
(fixes are staged automatically) and validates the message; on push it runs the Go
tests and the frontend typecheck. To bypass in an emergency: `git commit --no-verify`.
