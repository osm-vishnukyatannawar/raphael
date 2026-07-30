# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Raphael is a Wails v2 desktop app (Go backend + React/TypeScript frontend in the OS
webview) that talks to the Pinestem REST API. `README.md` documents the *product*
and every API quirk in detail — read the relevant section before touching a feature.
This file covers what is needed to work in the repo without re-deriving it.

## Commands

```sh
make setup       # once per clone: tools, frontend deps, git hooks
make dev         # hot-reloading window
make build       # production binary into build/bin/
make test        # go test -race ./...
make lint        # golangci-lint + ESLint + Prettier
make fmt         # format Go and frontend in place
make typecheck   # frontend tsc -b
make generate    # sqlc queries + Wails TypeScript bindings
make install-desktop  # Linux: install binary, icon and Raphael.desktop
```

A single Go test:

```sh
go test ./internal/monitor/ -run TestRefreshAggregatesAcrossProjects -v
```

**Building by hand on Linux needs `-tags webkit2_41`** (`go build ./...` is fine;
`wails build` / `wails dev` are not). Wails defaults to the webkit2gtk-4.0 API that
current distros no longer ship. The Makefile adds the tag; ad-hoc `wails` commands
must too.

**Always run tests with `-race`.** A data race in `internal/db` passed a plain
`go test` and failed only in CI. Note `lefthook.yml`'s pre-push hook still runs
`go test ./...` without it, so pre-push is weaker than `make test` and CI.

## Regeneration — the two steps that are easy to forget

- **Changed `internal/db/queries/*.sql` or added a migration?** Run
  `go tool sqlc generate`. `internal/db/sqlc/` is generated output; never hand-edit.
- **Changed an exported method or a bound struct on `App`?** Run
  `wails generate module`. `frontend/wailsjs/` is generated but **committed**,
  because `tsc` and ESLint need it to resolve imports on a fresh clone. Treat churn
  there as noise, not review surface.

Migrations are goose, embedded, and applied on startup. They are append-only in
practice — the app is released, so edit a shipped migration only to fix something
that has never run anywhere.

## Architecture

The shape is the same for every feature, and copying it is usually the right move:

```
internal/pinestem/   REST client. One file per API area. Knows HTTP, not storage.
internal/<feature>/  Service: Cached(ctx) reads SQLite, Refresh(ctx) hits the API
                     and replaces the cache. Owns its domain logic.
app.go               Thin binding layer. Exported methods become TS bindings.
internal/poller/     Two goroutines that call the services on an interval.
```

**`app.go` stays thin.** It translates between the frontend and services, and turns
sentinel errors into flags the UI branches on (e.g. a wrong password comes back as
`SignInResult{InvalidLogin: true}`, not an error, so the form can render it inline).
Domain logic belongs in `internal/<feature>`.

**Services never import Wails.** `internal/tasks` returns which tasks are new;
`app.go` decides whether to notify. That keeps the services testable without a
desktop session. `internal/notify` is the only package wrapping the Wails runtime.

**Refresh loops live in Go, not the frontend.** WebKitGTK and Chromium throttle
timers in a hidden page, and the new-task alert exists to reach the user when the
window *isn't* in front. The pollers emit `tasks:updated`, `billing:updated` and
`monitors:updated`; the frontend subscribes with `EventsOn`/`EventsOff` and has no
`setInterval`.

**Caches are replaced wholesale inside one transaction** (`replaceCache` in
`internal/tasks` and `internal/billing`, `replaceActuals` in `internal/monitor`).
A row that disappears upstream must disappear locally, and a failed refresh must
leave the previous data intact rather than blanking the screen. Every result type
carries `FromCacheOnly` + `ErrorMessage` so the UI can warn over stale data.

**Time is stored as minutes, formatted as hours at the edge.** Pinestem reports
integer minutes and 0.1h has no exact binary float representation.

### Adding a feature end to end

1. Migration in `internal/db/migrations/`, queries in `internal/db/queries/`
2. `go tool sqlc generate`
3. Client method in `internal/pinestem/` + tests against captured payloads
4. Service in `internal/<feature>/` + tests using `db.OpenAt` on a temp file
5. Bound method on `App`, then `wails generate module`
6. Frontend page/component under `frontend/src/`

## Pinestem API landmines

Each of these caused a real bug. `README.md` has the full detail.

- **A failed login returns HTTP 200 with `Status: false`** — identical to success,
  with an empty `ErrorMessage`. The only signal is a non-empty
  `MultipleResults[0].TokenId`.
- **`EmpID` on the billing endpoints is the whose-hours filter.** Omit it and you
  get the *whole team*, silently and with no error. `CallerID` (who is asking) is a
  separate field; conflating them is how the personal figures were first wrong.
- **`PageLimit` is capped at 100 server-side.** `billingPageLimit` must equal the
  cap — set it higher and a full page reads as "short, therefore last", and later
  pages are dropped into a quietly wrong total.
- **`BillableHours` / `NonBillableHours` are minutes**, despite the names. Per-row
  `TotalHours` is always `0`; compute billable + non-billable.
- **`UserId` is per company.** The same person has a different ID in each company;
  always use the one from the stored auth response.
- **`TaskStatusID = 4063`** ("In review") is hardcoded — no status-lookup endpoint
  exists. Verified for company 453.
- **Pinestem's `TimeZone` is a *Windows* identifier** (`"India Standard Time"`),
  which Go's `time.LoadLocation` cannot resolve. Day boundaries use `time.Local`;
  the string is still forwarded to the API, which expects that form.

**Never commit captured API payloads verbatim.** A live token and colleagues' names
reached the public repo that way. Use fabricated IDs and names in fixtures; the
payload *shape* is what the tests are for.

## Platform notes

- **Wayland will not let an app focus itself on demand.** `notify.Raise` maximises
  and presents; whether focus follows is the compositor's call. `WindowShow` is
  `gtk_widget_show` (maps only — does not raise); `WindowUnminimise` is
  `gtk_window_present` and must come last. `WindowSetAlwaysOnTop` is an X11 hint
  Wayland ignores.
- **Notifications bypass Wails on Linux** (`internal/notify/dbus_linux.go`) because
  Wails hardcodes the D-Bus `expire_timeout` to `-1`. `0` means "until dismissed",
  the opposite of what `0` means for the refresh intervals.
- **`Raphael.desktop` must match the GTK program name** set in `main.go`, which
  becomes the Wayland `app_id`. Mismatch means a generic window icon and no
  notification-activated raise.
- `wails doctor` reporting `libwebkit — Not Found` on Arch is a false negative.

## Conventions

Conventional Commits, enforced by commitlint. **Scopes are restricted** to:
`db api sync ui ai config build ci deps repo` (`.commitlintrc.json`).

Lefthook runs formatters and linters on commit and tests on push; `make setup`
installs it.

Releases are tag-driven — pushing `v*` runs `.github/workflows/release.yml`, which
re-runs tests, builds both platforms on their own runners (no cross-compiling: Wails
needs cgo), and publishes archives with checksums.

## Things that look wrong but are deliberate

- `frontend/dist/.gitkeep` is tracked and rewritten by a Vite plugin. Without it
  `//go:embed all:frontend/dist` fails on a fresh clone before any frontend build.
- Path aliases are declared in *both* `vite.config.ts` and `tsconfig.app.json`, and
  duplicated in `tsconfig.json` because the shadcn CLI reads that file.
- `internal/ai/`, `internal/api/` and `internal/sync/` are empty scaffold
  directories.
