//go:build !linux

package notify

import (
	"context"
	"fmt"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// platform uses the Wails runtime, which on Windows raises a toast.
//
// Unlike the Linux path this cannot honour Notification.Timeout: Wails exposes
// no duration, and the Windows toast API only offers "short" and "long"
// presets rather than a duration anyway. The setting is therefore Linux-only in
// practice, and the settings dialog says so.
type platform struct {
	onAction func()
}

func newPlatform(onAction func()) (*platform, error) {
	return &platform{onAction: onAction}, nil
}

func (p *platform) send(ctx context.Context, n Notification) error {
	if err := wailsruntime.InitializeNotifications(ctx); err != nil {
		return fmt.Errorf("notify: initialize: %w", err)
	}

	err := wailsruntime.SendNotification(ctx, wailsruntime.NotificationOptions{
		ID:    n.ID,
		Title: n.Title,
		Body:  n.Body,
	})
	if err != nil {
		return fmt.Errorf("notify: send notification: %w", err)
	}

	return nil
}

func (p *platform) close(ctx context.Context) {
	wailsruntime.CleanupNotifications(ctx)
}
