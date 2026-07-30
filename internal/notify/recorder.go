package notify

import (
	"context"
	"sync"
)

// Recorder is a Notifier that records instead of alerting. Tests use it to
// assert what *would* have been shown without needing a desktop session.
type Recorder struct {
	mu     sync.Mutex
	sent   []Notification
	raises int
	err    error
}

// FailWith makes every Notify return err, exercising the caller's error path.
func (r *Recorder) FailWith(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.err = err
}

func (r *Recorder) Notify(_ context.Context, n Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return r.err
	}
	r.sent = append(r.sent, n)

	return nil
}

func (r *Recorder) Raise(context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.raises++
}

// Sent is a copy of the notifications recorded so far.
func (r *Recorder) Sent() []Notification {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Notification(nil), r.sent...)
}

// Raises is how many times the window would have been brought forward.
func (r *Recorder) Raises() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.raises
}
