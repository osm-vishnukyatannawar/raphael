package notify

import (
	"context"
	"fmt"
	"sync"

	"github.com/godbus/dbus/v5"
)

// The freedesktop notification service.
const (
	dbusService   = "org.freedesktop.Notifications"
	dbusPath      = "/org/freedesktop/Notifications"
	dbusInterface = "org.freedesktop.Notifications"
)

// defaultActionKey is the action invoked by clicking the notification body
// rather than a button. The daemon only treats it specially under this exact
// name.
const defaultActionKey = "default"

// platform talks to the notification daemon over D-Bus directly.
//
// This bypasses Wails' runtime.SendNotification, which hardcodes the Notify
// expire_timeout argument to -1 ("server default"). That is the whole reason
// the duration was not configurable: the value was never ours to set. Going
// direct also lets us set the desktop-entry hint and replace the previous
// notification instead of stacking.
//
// godbus is already in the dependency graph via Wails and go-keyring, so this
// adds no new module.
type platform struct {
	conn     *dbus.Conn
	signals  chan *dbus.Signal
	onAction func()

	mu sync.Mutex
	// lastID is the daemon's ID for the notification currently on screen,
	// passed back as replaces_id so a second alert updates it in place.
	lastID uint32
}

func newPlatform(onAction func()) (*platform, error) {
	p := &platform{onAction: onAction}

	conn, err := dbus.SessionBus()
	if err != nil {
		return p, fmt.Errorf("notify: connect to the session bus: %w", err)
	}
	p.conn = conn

	// Watch for the user clicking a notification.
	err = conn.AddMatchSignal(
		dbus.WithMatchObjectPath(dbusPath),
		dbus.WithMatchInterface(dbusInterface),
		dbus.WithMatchMember("ActionInvoked"),
	)
	if err != nil {
		return p, fmt.Errorf("notify: subscribe to notification actions: %w", err)
	}

	p.signals = make(chan *dbus.Signal, 8)
	conn.Signal(p.signals)
	go p.watch(p.signals)

	return p, nil
}

// watch turns ActionInvoked signals for our own notification into a callback.
func (p *platform) watch(signals <-chan *dbus.Signal) {
	for sig := range signals {
		if sig.Name != dbusInterface+".ActionInvoked" || len(sig.Body) < 2 {
			continue
		}

		id, ok := sig.Body[0].(uint32)
		if !ok {
			continue
		}

		p.mu.Lock()
		ours := id == p.lastID
		p.mu.Unlock()

		if ours && p.onAction != nil {
			p.onAction()
		}
	}
}

func (p *platform) send(_ context.Context, n Notification) error {
	if p.conn == nil {
		return fmt.Errorf("notify: no session bus connection")
	}

	// -1 would mean "let the daemon decide"; 0 means "until dismissed". Both
	// are meaningful, so the zero Timeout maps to 0 rather than -1.
	expiry := int32(n.Timeout.Milliseconds())

	hints := map[string]dbus.Variant{
		// Lets KDE associate the notification with this app: it supplies the
		// icon, and clicking through can activate the window. Requires
		// Raphael.desktop to be installed — see `make install-desktop`.
		"desktop-entry": dbus.MakeVariant(AppID),
		"urgency":       dbus.MakeVariant(byte(1)), // normal
	}

	// Giving the default action a label makes the whole notification clickable.
	actions := []string{defaultActionKey, "Open Raphael"}

	p.mu.Lock()
	replaces := p.lastID
	p.mu.Unlock()

	var id uint32
	call := p.conn.Object(dbusService, dbusPath).Call(
		dbusInterface+".Notify", 0,
		AppID, replaces, "raphael", n.Title, n.Body, actions, hints, expiry,
	)
	if err := call.Store(&id); err != nil {
		return fmt.Errorf("notify: send notification: %w", err)
	}

	p.mu.Lock()
	p.lastID = id
	p.mu.Unlock()

	return nil
}

func (p *platform) close(context.Context) {
	if p.conn == nil || p.signals == nil {
		return
	}

	// Deliberately not closing the connection: dbus.SessionBus returns a
	// process-wide shared handle, and go-keyring uses it too. Detaching our
	// channel ends the watch goroutine without disturbing anyone else.
	p.conn.RemoveSignal(p.signals)
	close(p.signals)
	p.signals = nil
}
