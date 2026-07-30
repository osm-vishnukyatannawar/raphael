package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"runtime"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/osm-vishnukyatannawar/raphael/internal/billing"
	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/notify"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/poller"
	"github.com/osm-vishnukyatannawar/raphael/internal/secret"
	"github.com/osm-vishnukyatannawar/raphael/internal/settings"
	"github.com/osm-vishnukyatannawar/raphael/internal/tasks"
)

// taskURLTemplate opens a task in the Pinestem web app. It is keyed by the
// task's short code (REST-2408), not its numeric TaskID.
const taskURLTemplate = "https://pinestem.com/dashboard.html#/tasks/%s/details/?companyId=%d"

// Events the backend emits. The frontend subscribes rather than polling, since
// the refresh loops live in Go — see internal/poller for why.
const (
	eventTasksUpdated   = "tasks:updated"
	eventBillingUpdated = "billing:updated"
)

// notificationID is reused for every new-task alert so the OS replaces the
// previous one instead of stacking a column of them.
const notificationID = "raphael-new-tasks"

// App is the root object bound to the frontend. Exported methods on App become
// callable from TypeScript via the generated bindings in frontend/wailsjs.
//
// Methods here stay thin: they translate between the frontend and the internal
// packages, and turn sentinel errors into flags the UI can branch on.
type App struct {
	ctx      context.Context
	version  string
	database *sql.DB
	identity *identity.Service
	tasks    *tasks.Service
	billing  *billing.Service
	settings *settings.Service
	notifier notify.Notifier
	// notifications is the concrete service, kept for its shutdown hook.
	notifications *notify.Service

	tasksPoller   *poller.Poller
	billingPoller *poller.Poller

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

// startup opens the database, probes the keyring, builds the services, and
// starts the refresh loops.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	notifier, err := notify.New()
	if err != nil {
		// Not fatal: a desktop without a notification daemon should still get a
		// working task list, just a quieter one.
		log.Printf("startup: notifications unavailable: %v", err)
	}
	// Clicking a notification raises the window. On Wayland this is the one
	// moment the compositor reliably allows it, because the click is a user
	// interaction — see notify.Service.Raise.
	notifier.OnActivate(func() { notifier.Raise(ctx) })
	a.notifier = notifier
	a.notifications = notifier

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
	a.billing = billing.New(database, client, a.identity, a.settings)

	a.startPollers(ctx)
}

// startPollers arms both refresh loops at their stored cadences.
func (a *App) startPollers(ctx context.Context) {
	current, err := a.settings.Get(ctx)
	if err != nil {
		log.Printf("startup: read settings: %v", err)
		current = ptr(settings.Defaults())
	}

	a.tasksPoller = poller.New(seconds(current.RefreshIntervalSeconds), a.syncTasks)
	a.billingPoller = poller.New(seconds(current.BillingRefreshIntervalSeconds), a.syncBilling)

	a.tasksPoller.Start(ctx)
	a.billingPoller.Start(ctx)

	// Paint live data on launch rather than making the user wait out a whole
	// interval on top of whatever the cache last held.
	a.tasksPoller.Trigger()
	a.billingPoller.Trigger()
}

// shutdown stops the loops and closes the database on window close.
func (a *App) shutdown(ctx context.Context) {
	if a.tasksPoller != nil {
		a.tasksPoller.Stop()
	}
	if a.billingPoller != nil {
		a.billingPoller.Stop()
	}

	if a.notifications != nil {
		a.notifications.Close(ctx)
	}

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

	// A newly signed-in account has its own queue and its own hours.
	a.tasksPoller.Trigger()
	a.billingPoller.Trigger()

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

// BillingResult is the billing equivalent of TasksResult.
type BillingResult struct {
	Summary       *billing.Summary `json:"summary"`
	SyncedAt      string           `json:"syncedAt"`
	ErrorMessage  string           `json:"errorMessage"`
	FromCacheOnly bool             `json:"fromCacheOnly"`
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

	return TasksResult{Tasks: list, SyncedAt: a.stamp(func(s *settings.Settings) string {
		return s.TasksSyncedAt
	})}
}

// RefreshTasks runs a refresh on demand and returns the result directly, so the
// button that called it can stop spinning without waiting for the event.
func (a *App) RefreshTasks() TasksResult {
	if a.tasks == nil {
		return TasksResult{Tasks: []tasks.Task{}, ErrorMessage: "Raphael is not ready yet."}
	}

	return a.refreshTasks(a.ctx)
}

// GetBilling returns the cached billing summary without hitting the network.
func (a *App) GetBilling() BillingResult {
	if a.billing == nil {
		return BillingResult{ErrorMessage: "Raphael is not ready yet."}
	}

	summary, err := a.billing.Cached(a.ctx)
	if err != nil {
		log.Printf("get billing: %v", err)

		return BillingResult{ErrorMessage: err.Error()}
	}

	return BillingResult{Summary: summary, SyncedAt: a.stamp(func(s *settings.Settings) string {
		return s.BillingSyncedAt
	})}
}

// RefreshBilling pulls live billing figures on demand.
func (a *App) RefreshBilling() BillingResult {
	if a.billing == nil {
		return BillingResult{ErrorMessage: "Raphael is not ready yet."}
	}

	return a.refreshBilling(a.ctx)
}

// GetBillingForDate answers a one-off lookup for any date, bypassing the cache.
func (a *App) GetBillingForDate(day string) (*billing.DayTotal, error) {
	if a.billing == nil {
		return nil, errors.New("raphael is not ready yet")
	}

	return a.billing.ForDate(a.ctx, day)
}

// GetSettings returns the stored preferences.
func (a *App) GetSettings() (*settings.Settings, error) {
	if a.settings == nil {
		return nil, errors.New("raphael is not ready yet")
	}

	return a.settings.Get(a.ctx)
}

// SaveSettings stores the preferences and returns them as actually persisted —
// intervals are clamped, so this may differ from what was sent.
//
// Both pollers are re-armed here rather than on the next tick, so turning the
// interval down takes effect immediately instead of after the old one expires.
func (a *App) SaveSettings(in settings.Settings) (*settings.Settings, error) {
	if a.settings == nil {
		return nil, errors.New("raphael is not ready yet")
	}

	saved, err := a.settings.Save(a.ctx, in)
	if err != nil {
		return nil, err
	}

	a.tasksPoller.Reload(seconds(saved.RefreshIntervalSeconds))
	a.billingPoller.Reload(seconds(saved.BillingRefreshIntervalSeconds))

	return saved, nil
}

// MarkTasksSeen clears every new-task highlight.
func (a *App) MarkTasksSeen() error {
	if a.tasks == nil {
		return errors.New("raphael is not ready yet")
	}

	return a.tasks.AcknowledgeAll(a.ctx)
}

// OpenTask opens a task in the user's default browser and clears its highlight
// — opening it is exactly what "I've seen this" means.
func (a *App) OpenTask(taskID int64, shortCode string) error {
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

	if err := a.tasks.Acknowledge(a.ctx, taskID); err != nil {
		// Worth logging, not worth refusing to open the task over.
		log.Printf("open task: acknowledge %d: %v", taskID, err)
	}

	wailsruntime.BrowserOpenURL(
		a.ctx,
		fmt.Sprintf(taskURLTemplate, url.PathEscape(shortCode), creds.CompanyID),
	)

	return nil
}

// syncTasks is the poller's callback: refresh, alert on anything new, then tell
// the frontend to repaint.
func (a *App) syncTasks(ctx context.Context) {
	result := a.refreshTasks(ctx)
	wailsruntime.EventsEmit(ctx, eventTasksUpdated, result)
}

func (a *App) syncBilling(ctx context.Context) {
	result := a.refreshBilling(ctx)
	wailsruntime.EventsEmit(ctx, eventBillingUpdated, result)
}

// refreshTasks does the work behind both the poller and the manual button.
//
// On failure it still returns whatever is cached, flagged FromCacheOnly, so a
// dropped network shows a warning over a stale list rather than an empty page.
func (a *App) refreshTasks(ctx context.Context) TasksResult {
	syncedAt := func() string {
		return a.stamp(func(s *settings.Settings) string { return s.TasksSyncedAt })
	}

	result, err := a.tasks.Refresh(ctx)
	if err != nil {
		if errors.Is(err, identity.ErrNotOnboarded) {
			// Expected before sign-in; the poller is already running by then.
			return TasksResult{Tasks: []tasks.Task{}}
		}
		log.Printf("refresh tasks: %v", err)

		cached, cacheErr := a.tasks.Cached(ctx)
		if cacheErr != nil {
			cached = []tasks.Task{}
		}

		return TasksResult{
			Tasks:         cached,
			SyncedAt:      syncedAt(),
			ErrorMessage:  err.Error(),
			FromCacheOnly: true,
		}
	}

	a.alert(ctx, result.New)

	return TasksResult{Tasks: result.Tasks, SyncedAt: syncedAt()}
}

func (a *App) refreshBilling(ctx context.Context) BillingResult {
	syncedAt := func() string {
		return a.stamp(func(s *settings.Settings) string { return s.BillingSyncedAt })
	}

	summary, err := a.billing.Refresh(ctx)
	if err != nil {
		if errors.Is(err, identity.ErrNotOnboarded) {
			return BillingResult{}
		}
		log.Printf("refresh billing: %v", err)

		cached, cacheErr := a.billing.Cached(ctx)
		if cacheErr != nil {
			cached = nil
		}

		return BillingResult{
			Summary:       cached,
			SyncedAt:      syncedAt(),
			ErrorMessage:  err.Error(),
			FromCacheOnly: true,
		}
	}

	return BillingResult{Summary: summary, SyncedAt: syncedAt()}
}

// alert notifies and raises the window for newly arrived tasks, each gated on
// its own setting so the two can be turned off independently.
func (a *App) alert(ctx context.Context, arrived []tasks.Task) {
	if len(arrived) == 0 {
		return
	}

	current, err := a.settings.Get(ctx)
	if err != nil {
		log.Printf("alert: read settings: %v", err)

		return
	}

	if current.NotifyNewTasks {
		headline, body := tasks.AlertText(arrived)
		timeout, _ := current.NotificationTimeout()

		err := a.notifier.Notify(ctx, notify.Notification{
			ID:      notificationID,
			Title:   "Raphael",
			Body:    headline + "\n" + body,
			Timeout: timeout,
		})
		if err != nil {
			log.Printf("alert: %v", err)
		}
	}

	if current.FocusOnNewTask {
		a.notifier.Raise(ctx)
	}
}

// stamp reads one of the sync timestamps, or "" if settings can't be read.
func (a *App) stamp(pick func(*settings.Settings) string) string {
	current, err := a.settings.Get(a.ctx)
	if err != nil {
		return ""
	}

	return pick(current)
}

func seconds(n int64) time.Duration {
	return time.Duration(n) * time.Second
}

func ptr[T any](v T) *T {
	return &v
}
