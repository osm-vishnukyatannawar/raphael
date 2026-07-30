// Package notify wraps the platform-specific parts of alerting — desktop
// notifications and pulling the window to the front.
//
// It exists so the rest of the app can decide *whether* to alert without
// importing the Wails runtime, and so the platform quirks documented here and
// in dbus_linux.go live in one place rather than being rediscovered.
package notify

import (
	"context"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppID is the GTK program name set in main.go, which becomes the Wayland
// app_id and the X11 WM_CLASS. The installed desktop entry must be named
// AppID + ".desktop" for KDE to connect a notification back to this window.
const AppID = "Raphael"

// Notification is one alert.
type Notification struct {
	// ID is stable across alerts so the daemon replaces the previous one
	// instead of stacking a column of them.
	ID    string
	Title string
	Body  string
	// Timeout is how long it stays on screen. Zero means until dismissed.
	Timeout time.Duration
}

// Notifier is the seam. Service is the real implementation; tests use Recorder.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
	Raise(ctx context.Context)
}

// Service alerts through the platform notification system.
type Service struct {
	platform *platform

	// onActivate runs when the user clicks a notification. Clicking is a user
	// interaction, which is the one moment a Wayland compositor will reliably
	// let an app raise itself — see Raise.
	onActivate func()
}

// New builds a Service. A failure to reach the notification system is returned
// but not fatal: the window still works, it is just quieter.
func New() (*Service, error) {
	s := &Service{}

	p, err := newPlatform(func() {
		if s.onActivate != nil {
			s.onActivate()
		}
	})
	s.platform = p

	return s, err
}

// OnActivate registers what to do when a notification is clicked.
func (s *Service) OnActivate(f func()) {
	s.onActivate = f
}

func (s *Service) Notify(ctx context.Context, n Notification) error {
	return s.platform.send(ctx, n)
}

// Close releases the notification connection on shutdown.
func (s *Service) Close(ctx context.Context) {
	s.platform.close(ctx)
}

// Raise brings the window to the foreground, maximised.
//
// The order matters. WindowShow is gtk_widget_show, which only maps the window;
// it neither raises nor focuses. The call that does both is WindowUnminimise,
// which is gtk_window_present underneath — so it has to come last, after the
// window is mapped and maximised, or the maximise undoes the raise.
//
// WindowSetAlwaysOnTop is gtk_window_set_keep_above. That is an X11 window
// manager hint and Wayland ignores it outright, so the brief pulse below helps
// on X11 and XWayland and does nothing on a native Wayland session. It is kept
// because it costs nothing and X11 is still common.
//
// On Wayland this may not focus the window at all. Activation there requires an
// xdg-activation token, which is only issued off the back of user interaction,
// and a background alert has none — the compositor decides, not the app. KDE
// will typically mark the window as demanding attention instead. The reliable
// routes are clicking the notification (see OnActivate) or a KWin window rule;
// both are covered in the README.
func (s *Service) Raise(ctx context.Context) {
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
	wailsruntime.WindowMaximise(ctx)

	wailsruntime.WindowSetAlwaysOnTop(ctx, true)
	time.Sleep(150 * time.Millisecond)
	wailsruntime.WindowSetAlwaysOnTop(ctx, false)

	// Present again, now that the window is mapped and maximised.
	wailsruntime.WindowUnminimise(ctx)
}
