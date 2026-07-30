// Package notify wraps the platform-specific parts of alerting — desktop
// notifications and pulling the window to the front.
//
// It exists so that the rest of the app can decide *whether* to alert without
// importing the Wails runtime, and so the two platform quirks documented below
// live in one file rather than being rediscovered.
package notify

import (
	"context"
	"fmt"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Notification is one alert.
type Notification struct {
	// ID lets the OS replace an earlier alert instead of stacking a new one.
	ID    string
	Title string
	Body  string
}

// Notifier is the seam. Wails is the real implementation; tests use Recorder.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
	Raise(ctx context.Context)
}

// Wails alerts through the Wails v2 runtime: D-Bus on Linux, toast notifications
// on Windows. No third-party dependency is involved.
type Wails struct{}

// Initialize must run before any notification is sent. On Linux it opens the
// D-Bus connection; on Windows it registers the app's toast activator.
func (Wails) Initialize(ctx context.Context) error {
	if err := wailsruntime.InitializeNotifications(ctx); err != nil {
		return fmt.Errorf("notify: initialize: %w", err)
	}

	return nil
}

// Cleanup releases the D-Bus connection on shutdown.
func (Wails) Cleanup(ctx context.Context) {
	wailsruntime.CleanupNotifications(ctx)
}

func (Wails) Notify(ctx context.Context, n Notification) error {
	err := wailsruntime.SendNotification(ctx, wailsruntime.NotificationOptions{
		ID:    n.ID,
		Title: n.Title,
		Body:  n.Body,
	})
	if err != nil {
		return fmt.Errorf("notify: send: %w", err)
	}

	return nil
}

// Raise brings the window to the foreground.
//
// The always-on-top pulse is not decoration. Most window managers implement
// focus-stealing prevention, which downgrades a plain Show from "raise" to
// "flash in the taskbar"; briefly setting the window above others is the one
// approach that reliably gets past it. It is set back immediately so the window
// does not stay pinned over everything else.
//
// Under KDE on Wayland this may still be refused — the compositor, not the app,
// has the final say. That is the same class of limitation as the window icon.
func (Wails) Raise(ctx context.Context) {
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.WindowShow(ctx)

	wailsruntime.WindowSetAlwaysOnTop(ctx, true)
	time.Sleep(120 * time.Millisecond)
	wailsruntime.WindowSetAlwaysOnTop(ctx, false)
}
