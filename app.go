package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/secret"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
	"github.com/osm-vishnukyatannawar/raphael/internal/tasks"
)

// taskURLTemplate opens a task in the Pinestem web app. It is keyed by the
// task's short code (REST-2408), not its numeric TaskID.
const taskURLTemplate = "https://pinestem.com/dashboard.html#/tasks/%s/details/?companyId=%d"

// App is the root object bound to the frontend. Exported methods on App become
// callable from TypeScript via the generated bindings in frontend/wailsjs.
//
// Methods here stay thin: they translate between the frontend and
// internal/identity, and turn sentinel errors into flags the UI can branch on.
type App struct {
	ctx      context.Context
	version  string
	database *sql.DB
	identity *identity.Service
	tasks    *tasks.Service
	settings *settings.Service

	// startupErr is recorded rather than fatal: a window that explains it can't
	// open its database is better than one that vanishes on launch.
	startupErr error
}

// AppInfo is static information about the running build.
type AppInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Platform string `json:"platform"`
}

// BootstrapResult is the first thing the frontend asks for. Onboarded decides
// which screen renders; Session is nil until onboarding completes.
type BootstrapResult struct {
	Info      AppInfo           `json:"info"`
	Onboarded bool              `json:"onboarded"`
	Session   *identity.Session `json:"session"`
	// Error is a human-readable startup failure (e.g. the database could not be
	// opened). Empty on a healthy launch.
	Error string `json:"error"`
}

// SignInResult separates "wrong password" from "something broke". The frontend
// shows the former inline on the form and the latter as a failure state.
type SignInResult struct {
	Session       *identity.Session `json:"session"`
	InvalidLogin  bool              `json:"invalidLogin"`
	ErrorMessage  string            `json:"errorMessage"`
	SuggestedName string            `json:"suggestedName"`
}

func NewApp(version string) *App {
	return &App{version: version}
}

// startup opens the database, probes the keyring, and builds the identity
// service. The context is held for the lifetime of the app.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	database, err := db.Open(ctx)
	if err != nil {
		log.Printf("startup: %v", err)
		a.startupErr = err

		return
	}
	a.database = database

	store := secret.Store(secret.Keyring{})
	keyringOK := secret.Available(store)
	if !keyringOK {
		log.Print("startup: OS keyring unavailable; the session token will be stored in SQLite " +
			"and the password will not be saved")
	}

	client := pinestem.New()
	a.identity = identity.New(database, client, store, keyringOK)
	a.settings = settings.New(database)
	a.tasks = tasks.New(database, client, a.identity, a.settings)
}

// shutdown closes the database on window close.
func (a *App) shutdown(_ context.Context) {
	if a.database != nil {
		if err := a.database.Close(); err != nil {
			log.Printf("shutdown: close database: %v", err)
		}
	}
}

// Info reports basic runtime details.
func (a *App) Info() AppInfo {
	return AppInfo{
		Name:     "Raphael",
		Version:  a.version,
		Platform: runtime.GOOS,
	}
}

// Bootstrap tells the frontend whether to onboard or go straight to the app.
func (a *App) Bootstrap() BootstrapResult {
	result := BootstrapResult{Info: a.Info()}

	if a.startupErr != nil {
		result.Error = a.startupErr.Error()

		return result
	}

	session, err := a.identity.Load(a.ctx)
	if errors.Is(err, identity.ErrNotOnboarded) {
		return result
	}
	if err != nil {
		log.Printf("bootstrap: %v", err)
		result.Error = err.Error()

		return result
	}

	result.Onboarded = true
	result.Session = session

	return result
}

// SignIn authenticates against Pinestem and stores the session.
//
// A wrong password is not an error return: it comes back as InvalidLogin so the
// frontend can render it on the form rather than as a crash.
func (a *App) SignIn(username, password string) SignInResult {
	if a.identity == nil {
		return SignInResult{ErrorMessage: "Raphael is not ready yet — check the startup error."}
	}

	session, err := a.identity.SignIn(a.ctx, username, password)
	switch {
	case errors.Is(err, pinestem.ErrInvalidCredentials):
		return SignInResult{InvalidLogin: true}
	case err != nil:
		// Deliberately logging err, not the credentials; the client keeps the
		// password out of its error strings.
		log.Printf("signin: %v", err)

		return SignInResult{ErrorMessage: err.Error()}
	}

	return SignInResult{Session: session, SuggestedName: session.DisplayName}
}

// SetDisplayName stores the name Raphael addresses the user by.
func (a *App) SetDisplayName(name string) error {
	if a.identity == nil {
		return errors.New("raphael is not ready yet")
	}

	return a.identity.SetDisplayName(a.ctx, name)
}

// SignOut clears the stored session and returns the app to onboarding.
func (a *App) SignOut() error {
	if a.identity == nil {
		return errors.New("raphael is not ready yet")
	}

	return a.identity.SignOut(a.ctx)
}

// TasksResult carries the list plus a refresh error, so a failed refresh can
// show a message *and* keep rendering the cached rows instead of blanking them.
type TasksResult struct {
	Tasks         []tasks.Task `json:"tasks"`
	SyncedAt      string       `json:"syncedAt"`
	ErrorMessage  string       `json:"errorMessage"`
	FromCacheOnly bool         `json:"fromCacheOnly"`
}

// ListTasks returns the cached in-review tasks without hitting the network.
func (a *App) ListTasks() TasksResult {
	if a.tasks == nil {
		return TasksResult{Tasks: []tasks.Task{}, ErrorMessage: "Raphael is not ready yet."}
	}

	list, err := a.tasks.Cached(a.ctx)
	if err != nil {
		log.Printf("list tasks: %v", err)

		return TasksResult{Tasks: []tasks.Task{}, ErrorMessage: err.Error()}
	}

	return TasksResult{Tasks: list, SyncedAt: a.syncedAt()}
}

// RefreshTasks pulls live data from Pinestem and updates the cache.
//
// On failure it still returns whatever is cached, flagged FromCacheOnly, so a
// dropped network shows a warning over a stale list rather than an empty page.
func (a *App) RefreshTasks() TasksResult {
	if a.tasks == nil {
		return TasksResult{Tasks: []tasks.Task{}, ErrorMessage: "Raphael is not ready yet."}
	}

	list, err := a.tasks.Refresh(a.ctx)
	if err != nil {
		log.Printf("refresh tasks: %v", err)

		cached, cacheErr := a.tasks.Cached(a.ctx)
		if cacheErr != nil {
			cached = []tasks.Task{}
		}

		return TasksResult{
			Tasks:         cached,
			SyncedAt:      a.syncedAt(),
			ErrorMessage:  err.Error(),
			FromCacheOnly: true,
		}
	}

	return TasksResult{Tasks: list, SyncedAt: a.syncedAt()}
}

// GetSettings returns the stored preferences.
func (a *App) GetSettings() (*settings.Settings, error) {
	if a.settings == nil {
		return nil, errors.New("raphael is not ready yet")
	}

	return a.settings.Get(a.ctx)
}

// SaveSettings stores the refresh interval and returns the settings as actually
// persisted — the interval is clamped, so this may differ from what was sent.
func (a *App) SaveSettings(refreshIntervalSeconds int64) (*settings.Settings, error) {
	if a.settings == nil {
		return nil, errors.New("raphael is not ready yet")
	}

	if _, err := a.settings.SetRefreshInterval(a.ctx, refreshIntervalSeconds); err != nil {
		return nil, err
	}

	return a.settings.Get(a.ctx)
}

// OpenTask opens a task in the user's default browser.
func (a *App) OpenTask(shortCode string) error {
	if a.identity == nil {
		return errors.New("raphael is not ready yet")
	}
	if shortCode == "" {
		return errors.New("task has no short code")
	}

	creds, err := a.identity.Credentials(a.ctx)
	if err != nil {
		return err
	}

	wailsruntime.BrowserOpenURL(
		a.ctx,
		fmt.Sprintf(taskURLTemplate, url.PathEscape(shortCode), creds.CompanyID),
	)

	return nil
}

// syncedAt reports when the cache was last refreshed, or "" if never.
func (a *App) syncedAt() string {
	current, err := a.settings.Get(a.ctx)
	if err != nil {
		return ""
	}

	return current.TasksSyncedAt
}
