package identity_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/secret"
)

// stubAuth stands in for the Pinestem client so these tests need no HTTP server.
type stubAuth struct {
	account *pinestem.Account
	err     error

	gotUser, gotPass string
}

func (s *stubAuth) Authenticate(_ context.Context, username, password string) (*pinestem.Account, error) {
	s.gotUser, s.gotPass = username, password
	if s.err != nil {
		return nil, s.err
	}

	return s.account, nil
}

func testAccount() *pinestem.Account {
	return &pinestem.Account{
		UserID:      6406,
		UserName:    "someone@example.com",
		FirstName:   "Venkata Krishna Dinesh",
		LastName:    "Madireddy",
		Token:       "tok-abc",
		CompanyID:   453,
		CompanyName: "Osmosys",
		RoleID:      2294,
		AccountType: "premium",
		TimeZone:    "India Standard Time",
	}
}

// newService wires a real (temp-file) database to a stub authenticator and an
// in-memory secret store. keyringOK mirrors production's startup probe.
func newService(t *testing.T, auth identity.Authenticator, keyringOK bool) (*identity.Service, *secret.Memory, *sql.DB) {
	t.Helper()

	conn, err := db.OpenAt(t.Context(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	store := secret.NewMemory()

	return identity.New(conn, auth, store, keyringOK), store, conn
}

func TestLoadBeforeOnboardingReportsNotOnboarded(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t, &stubAuth{account: testAccount()}, true)

	if _, err := svc.Load(t.Context()); !errors.Is(err, identity.ErrNotOnboarded) {
		t.Fatalf("err = %v, want ErrNotOnboarded", err)
	}
}

func TestSignInPersistsSessionAndSecrets(t *testing.T) {
	t.Parallel()

	auth := &stubAuth{account: testAccount()}
	svc, store, _ := newService(t, auth, true)

	session, err := svc.SignIn(t.Context(), "  someone@example.com  ", "hunter2")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	if auth.gotUser != "someone@example.com" {
		t.Errorf("username not trimmed before use: %q", auth.gotUser)
	}
	// Display name seeds from the Pinestem name so onboarding can prefill it.
	if session.DisplayName != "Venkata Krishna Dinesh Madireddy" {
		t.Errorf("DisplayName = %q", session.DisplayName)
	}
	if session.CompanyName != "Osmosys" || session.CompanyID != 453 {
		t.Errorf("company = %d/%q", session.CompanyID, session.CompanyName)
	}
	if !session.SecretsInKeyring {
		t.Error("SecretsInKeyring = false, want true when the keyring works")
	}

	if got, _ := store.Get(secret.KeyPinestemToken); got != "tok-abc" {
		t.Errorf("token in keyring = %q, want tok-abc", got)
	}
	if got, _ := store.Get(secret.KeyPinestemPassword); got != "hunter2" {
		t.Errorf("password in keyring = %q", got)
	}

	// The session handed to the frontend must not carry the token.
	creds, err := svc.Credentials(t.Context())
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}
	if creds.Token != "tok-abc" || creds.CompanyID != 453 {
		t.Errorf("credentials = %+v", creds)
	}
}

// With a working keyring the token must never be written to SQLite.
func TestSignInKeepsTokenOutOfDatabase(t *testing.T) {
	t.Parallel()

	svc, _, conn := newService(t, &stubAuth{account: testAccount()}, true)

	if _, err := svc.SignIn(t.Context(), "u", "p"); err != nil {
		t.Fatalf("SignIn: %v", err)
	}

	var fallback *string
	err := conn.QueryRowContext(t.Context(), "SELECT token_fallback FROM pinestem_account WHERE id = 1").
		Scan(&fallback)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if fallback != nil {
		t.Errorf("token_fallback = %q, want NULL when the keyring is available", *fallback)
	}
}

// Without a keyring the app must still work: token to SQLite, password nowhere.
func TestSignInFallsBackWhenKeyringUnavailable(t *testing.T) {
	t.Parallel()

	svc, store, conn := newService(t, &stubAuth{account: testAccount()}, false)

	session, err := svc.SignIn(t.Context(), "u", "hunter2")
	if err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if session.SecretsInKeyring {
		t.Error("SecretsInKeyring = true, want false in the degraded path")
	}

	if store.Has(secret.KeyPinestemPassword) {
		t.Error("password was stored despite there being no keyring")
	}

	var fallback *string
	if err := conn.QueryRowContext(t.Context(),
		"SELECT token_fallback FROM pinestem_account WHERE id = 1").Scan(&fallback); err != nil {
		t.Fatalf("query: %v", err)
	}
	if fallback == nil || *fallback != "tok-abc" {
		t.Errorf("token_fallback = %v, want tok-abc", fallback)
	}

	creds, err := svc.Credentials(t.Context())
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}
	if creds.Token != "tok-abc" {
		t.Errorf("Credentials.Token = %q", creds.Token)
	}
}

func TestSignInFailureWritesNothing(t *testing.T) {
	t.Parallel()

	auth := &stubAuth{err: pinestem.ErrInvalidCredentials}
	svc, store, conn := newService(t, auth, true)

	if _, err := svc.SignIn(t.Context(), "u", "wrong"); !errors.Is(err, pinestem.ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}

	if store.Has(secret.KeyPinestemToken) || store.Has(secret.KeyPinestemPassword) {
		t.Error("secrets written despite failed sign-in")
	}

	var n int
	if err := conn.QueryRowContext(t.Context(), "SELECT count(*) FROM pinestem_account").Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 0 {
		t.Errorf("pinestem_account rows = %d, want 0", n)
	}
}

func TestSetDisplayNameOverridesTheSeededName(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t, &stubAuth{account: testAccount()}, true)

	if _, err := svc.SignIn(t.Context(), "u", "p"); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if err := svc.SetDisplayName(t.Context(), "  Dinesh  "); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}

	session, err := svc.Load(t.Context())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if session.DisplayName != "Dinesh" {
		t.Errorf("DisplayName = %q, want Dinesh (trimmed)", session.DisplayName)
	}
}

func TestSetDisplayNameRejectsBlank(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t, &stubAuth{account: testAccount()}, true)

	if err := svc.SetDisplayName(t.Context(), "   "); !errors.Is(err, identity.ErrNameRequired) {
		t.Fatalf("err = %v, want ErrNameRequired", err)
	}
}

// Re-authenticating (e.g. after a dead token) must not reset a chosen name.
func TestSignInPreservesChosenDisplayName(t *testing.T) {
	t.Parallel()

	svc, _, _ := newService(t, &stubAuth{account: testAccount()}, true)

	if _, err := svc.SignIn(t.Context(), "u", "p"); err != nil {
		t.Fatalf("first SignIn: %v", err)
	}
	if err := svc.SetDisplayName(t.Context(), "Dinesh"); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}

	session, err := svc.SignIn(t.Context(), "u", "p")
	if err != nil {
		t.Fatalf("second SignIn: %v", err)
	}
	if session.DisplayName != "Dinesh" {
		t.Errorf("DisplayName = %q, want the chosen name to survive re-auth", session.DisplayName)
	}
}

func TestSignOutClearsEverything(t *testing.T) {
	t.Parallel()

	svc, store, _ := newService(t, &stubAuth{account: testAccount()}, true)

	if _, err := svc.SignIn(t.Context(), "u", "p"); err != nil {
		t.Fatalf("SignIn: %v", err)
	}
	if err := svc.SignOut(t.Context()); err != nil {
		t.Fatalf("SignOut: %v", err)
	}

	if store.Has(secret.KeyPinestemToken) || store.Has(secret.KeyPinestemPassword) {
		t.Error("keyring entries survived sign-out")
	}
	if _, err := svc.Load(t.Context()); !errors.Is(err, identity.ErrNotOnboarded) {
		t.Errorf("Load after sign-out = %v, want ErrNotOnboarded", err)
	}
}
