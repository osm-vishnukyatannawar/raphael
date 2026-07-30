# Raphael

Cross-platform desktop app (Linux + Windows) built with [Wails v2](https://wails.io):
a Go backend bound to a React + TypeScript frontend running in the OS webview.

It signs in to Pinestem, shows the tasks waiting on your review, alerts you when a
new one arrives, and tracks the hours you've billed today and this week.

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
internal/config/         Resolves ~/.config/raphael (%AppData%\raphael on Windows)
internal/db/             SQLite connection + goose migrations (embedded)
  migrations/            Schema, applied on startup
  queries/               sqlc input
  sqlc/                  sqlc output — generated, do not edit
internal/pinestem/       Pinestem REST client (auth, tasks, billing)
internal/identity/       Onboarding state: display name + Pinestem session
internal/secret/         OS keyring access
internal/tasks/          In-review queue: cache, new-task detection, alert wording
internal/billing/        Logged hours: per-day cache, week arithmetic
internal/settings/       Preferences (intervals, week start, alert toggles)
internal/poller/         The two refresh loops — see "Refresh loops" below
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

Both refresh loops are **Go goroutines** (`internal/poller`), not `setInterval` in
the frontend. WebKitGTK and Chromium both throttle timers in a hidden page —
Chromium down to one wake per minute after five minutes — and the new-task alert
exists precisely to reach you when the window *isn't* in front. The backend emits
`tasks:updated` / `billing:updated`; the frontend subscribes.

Tasks and billing have separate intervals (60s and 300s by default) and separate
on-demand refresh buttons. `0` disables either one; anything under 15s is raised to
15s.

## Tasks in review

The main screen lists the tasks assigned to you that are in "In review",
newest-modified first. Rows open the task in your browser.

Two calls per refresh, in order:

1. `GET Projects/ProjectsDropdown` → every project you can read
2. `GET Tasks/Filter` with one repeated `ProjectCode` per project

Projects are fetched live rather than hardcoded because they get added and removed.

### New-task alerts

A task that wasn't in the queue on the previous refresh triggers a desktop
notification and pulls the window forward, each toggleable in settings. The row stays
highlighted until you open it or press "Mark all seen".

Notifications use the **Wails v2.13 runtime** (`runtime.SendNotification`) — D-Bus on
Linux, toast on Windows. No third-party notification dependency.

Raising the window sets always-on-top for ~120ms and then clears it. That looks odd
but is deliberate: a plain `WindowShow` is downgraded to a taskbar flash by most
window managers' focus-stealing prevention. KDE on Wayland may refuse anyway — the
compositor has the final say, same as with the window icon.

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
  to the UI; 0.1h has no exact binary float representation.
- **Per-row `TotalHours` is always `0`.** The row total is billable + non-billable.

`ProjectIds` is optional; omitting it covers every project, so billing needs no
project list. `Reports/FilterDailyBillingDetails` appears in the bundle but 404s, and
`FilterBillingDetails_New` rejects GET.

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

**`TaskStatusID = 4063`** ("3. In review") is hardcoded in `internal/pinestem/tasks.go`.
Pinestem exposes no status-lookup endpoint — four plausible paths all return 404 — so
this is the single place to change if a company numbers statuses differently.

Tasks and projects are cached in SQLite and **replaced wholesale** on each refresh
inside one transaction: a task leaving the queue must disappear, and a mid-refresh
failure leaves the previous cache intact rather than blanking the list.

```sh
sqlite3 ~/.config/raphael/raphael.db \
  'SELECT short_code, project_code, modified_on FROM task ORDER BY modified_on DESC;'
```

## Icon

`build/appicon.svg` is the source of truth; `make icon` rasterizes it to
`build/appicon.png` and `build/windows/icon.ico`.

> On **KDE under Wayland** the window icon shows as a generic placeholder. That is not
> a build problem — KWin ignores the GTK window icon and resolves `app_id` against an
> installed `.desktop` file, which a dev build doesn't have. Verified working under
> `GDK_BACKEND=x11` and in the packaged Windows build.

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
