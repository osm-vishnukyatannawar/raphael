# Raphael

Cross-platform desktop app (Linux + Windows) built with [Wails v2](https://wails.io):
a Go backend bound to a React + TypeScript frontend running in the OS webview.

**Status: scaffold.** Tooling, structure, linting, and CI are in place. There is no
schema, no API client, and no real UI yet — the window renders a placeholder that
exercises the Go → TypeScript → Tailwind → shadcn pipeline end to end.

## Requirements

| | Version |
|---|---|
| Go | 1.25+ |
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
internal/api/            Remote REST client
internal/sync/           Background sync engine
internal/ai/             AI integration seam
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
