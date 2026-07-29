package main

import (
	"context"
	"runtime"
)

// App is the root object bound to the frontend. Exported methods on App become
// callable from TypeScript via the generated bindings in frontend/wailsjs.
type App struct {
	ctx     context.Context
	version string
}

// AppInfo is returned to the frontend on startup. Exported structs used in
// bound method signatures are emitted as TypeScript models by `wails generate`.
type AppInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

func NewApp(version string) *App {
	return &App{version: version}
}

// startup runs once the Wails runtime is ready. Long-lived services are wired
// here so they can use ctx for cancellation.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown runs on window close, before the process exits. Services started in
// startup are stopped here.
func (a *App) shutdown(_ context.Context) {}

// Info reports basic runtime details. It exists to keep the Go -> TypeScript
// binding pipeline exercised while the app has no real methods yet.
func (a *App) Info() AppInfo {
	return AppInfo{
		Name:     "Raphael",
		Version:  a.version,
		Platform: runtime.GOOS,
	}
}
