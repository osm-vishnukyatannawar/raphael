// Package secret stores credentials in the OS keyring.
//
// The Store interface is not incidental: CI runners have no Secret Service, and
// unit tests must never touch the developer's real keyring. Production code
// takes Keyring, tests take Memory.
package secret

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// service namespaces Raphael's entries in the OS credential store.
const service = "raphael"

// Keys used within the service.
const (
	KeyPinestemToken    = "pinestem-token"
	KeyPinestemPassword = "pinestem-password"
)

// ErrNotFound is returned when a key has no stored value.
var ErrNotFound = errors.New("secret: not found")

// Store is a small key/value credential store.
type Store interface {
	Set(key, value string) error
	Get(key string) (string, error)
	Delete(key string) error
}

// Keyring is the OS-backed Store: Secret Service on Linux (KWallet or GNOME
// Keyring), Credential Manager on Windows, Keychain on macOS.
type Keyring struct{}

func (Keyring) Set(key, value string) error {
	if err := keyring.Set(service, key, value); err != nil {
		return fmt.Errorf("secret: store %q: %w", key, err)
	}

	return nil
}

func (Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("secret: read %q: %w", key, err)
	}

	return v, nil
}

func (Keyring) Delete(key string) error {
	err := keyring.Delete(service, key)
	// Deleting something that was never stored is not a failure — sign-out runs
	// this unconditionally.
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}

	return fmt.Errorf("secret: delete %q: %w", key, err)
}

// Available reports whether the OS keyring actually works, by round-tripping a
// probe value. go-keyring only fails on first use, so this has to write rather
// than just inspect: on a headless box the D-Bus call errors here instead of
// midway through sign-in.
func Available(s Store) bool {
	const probe = "availability-probe"

	if err := s.Set(probe, "1"); err != nil {
		return false
	}
	defer func() { _ = s.Delete(probe) }()

	if _, err := s.Get(probe); err != nil {
		return false
	}

	return true
}
