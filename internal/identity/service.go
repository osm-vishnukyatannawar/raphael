// Package identity owns onboarding state: the local display name and the
// Pinestem session. It is the single place that decides where secrets live, so
// app.go stays a thin binding layer.
package identity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/db/sqlc"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/secret"
)

// ErrNotOnboarded is returned by Load when no Pinestem session is stored yet.
var ErrNotOnboarded = errors.New("identity: not onboarded")

// ErrNameRequired is returned when a blank display name is submitted.
var ErrNameRequired = errors.New("identity: display name is required")

// Authenticator is the slice of the Pinestem client this package needs. Narrow
// on purpose, so tests can substitute a stub without an HTTP server.
type Authenticator interface {
	Authenticate(ctx context.Context, username, password string) (*pinestem.Account, error)
}

// Session is what the frontend sees. It deliberately carries no token: the UI
// never needs it, so it should not be sitting in a JS heap or a devtools panel.
type Session struct {
	DisplayName      string `json:"displayName"`
	UserName         string `json:"userName"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	CompanyID        int64  `json:"companyId"`
	CompanyName      string `json:"companyName"`
	IsProjectManager bool   `json:"isProjectManager"`
	IsTeamLead       bool   `json:"isTeamLead"`
	// SecretsInKeyring is false when the OS keyring was unavailable and the
	// token had to be written to SQLite instead. The UI surfaces this.
	SecretsInKeyring bool `json:"secretsInKeyring"`
}

// Credentials is everything needed to call the Pinestem API on the user's
// behalf. UserID belongs here rather than on Session because it is a request
// parameter (Tasks/Filter's AssignedTo), not something the UI displays — and it
// is per-company, so it only means anything alongside CompanyID.
type Credentials struct {
	Token     string
	CompanyID int64
	UserID    int64
}

type Service struct {
	queries *sqlc.Queries
	auth    Authenticator
	store   secret.Store

	// keyringOK is probed once at construction. When false, the token is
	// persisted in SQLite and the password is not persisted at all.
	keyringOK bool
}

func New(database *sql.DB, auth Authenticator, store secret.Store, keyringOK bool) *Service {
	return &Service{
		queries:   sqlc.New(database),
		auth:      auth,
		store:     store,
		keyringOK: keyringOK,
	}
}

// SignIn authenticates against Pinestem and persists the result.
//
// The display name is seeded from the Pinestem name; onboarding immediately
// offers it for editing via SetDisplayName.
func (s *Service) SignIn(ctx context.Context, username, password string) (*Session, error) {
	account, err := s.auth.Authenticate(ctx, strings.TrimSpace(username), password)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Secrets first: if the keyring rejects the write we fall back before
	// touching the database, so the two can't disagree about where the token is.
	var tokenFallback *string
	if s.keyringOK {
		if err := s.store.Set(secret.KeyPinestemToken, account.Token); err != nil {
			return nil, err
		}
		if err := s.store.Set(secret.KeyPinestemPassword, password); err != nil {
			return nil, err
		}
	} else {
		token := account.Token
		tokenFallback = &token
	}

	err = s.queries.UpsertPinestemAccount(ctx, sqlc.UpsertPinestemAccountParams{
		UserID:           account.UserID,
		UserName:         account.UserName,
		FirstName:        account.FirstName,
		LastName:         account.LastName,
		CompanyID:        account.CompanyID,
		CompanyName:      account.CompanyName,
		RoleID:           account.RoleID,
		IsProjectManager: boolToInt(account.IsProjectManager),
		IsTeamLead:       boolToInt(account.IsTeamLead),
		AccountType:      account.AccountType,
		TimeZone:         account.TimeZone,
		DateTimeFormat:   account.DateTimeFormat,
		AuthenticatedAt:  now,
		TokenFallback:    tokenFallback,
	})
	if err != nil {
		return nil, fmt.Errorf("identity: persist account: %w", err)
	}

	// Seed the profile only if this is the first sign-in; a later re-auth must
	// not clobber a name the user chose.
	displayName := account.FullName()
	if existing, err := s.queries.GetProfile(ctx); err == nil {
		displayName = existing.DisplayName
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("identity: read profile: %w", err)
	} else if err := s.upsertProfile(ctx, displayName, now); err != nil {
		return nil, err
	}

	return s.sessionFrom(displayName, account), nil
}

// SetDisplayName updates the name Raphael addresses the user by.
func (s *Service) SetDisplayName(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNameRequired
	}

	return s.upsertProfile(ctx, name, time.Now().UTC().Format(time.RFC3339))
}

// Load returns the stored session, or ErrNotOnboarded on a fresh install.
func (s *Service) Load(ctx context.Context) (*Session, error) {
	account, err := s.queries.GetPinestemAccount(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotOnboarded
	}
	if err != nil {
		return nil, fmt.Errorf("identity: read account: %w", err)
	}

	displayName := strings.TrimSpace(account.FirstName + " " + account.LastName)
	profile, err := s.queries.GetProfile(ctx)
	switch {
	case err == nil:
		displayName = profile.DisplayName
	case errors.Is(err, sql.ErrNoRows):
		// Account without a profile: possible if onboarding was closed between
		// the two steps. Fall back to the Pinestem name rather than failing.
	default:
		return nil, fmt.Errorf("identity: read profile: %w", err)
	}

	return &Session{
		DisplayName:      displayName,
		UserName:         account.UserName,
		FirstName:        account.FirstName,
		LastName:         account.LastName,
		CompanyID:        account.CompanyID,
		CompanyName:      account.CompanyName,
		IsProjectManager: account.IsProjectManager == 1,
		IsTeamLead:       account.IsTeamLead == 1,
		SecretsInKeyring: account.TokenFallback == nil,
	}, nil
}

// Credentials returns the token and company for authenticated API calls,
// reading from wherever SignIn put the token.
func (s *Service) Credentials(ctx context.Context) (*Credentials, error) {
	account, err := s.queries.GetPinestemAccount(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotOnboarded
	}
	if err != nil {
		return nil, fmt.Errorf("identity: read account: %w", err)
	}

	if account.TokenFallback != nil {
		return &Credentials{
			Token:     *account.TokenFallback,
			CompanyID: account.CompanyID,
			UserID:    account.UserID,
		}, nil
	}

	token, err := s.store.Get(secret.KeyPinestemToken)
	if err != nil {
		return nil, err
	}

	return &Credentials{
		Token:     token,
		CompanyID: account.CompanyID,
		UserID:    account.UserID,
	}, nil
}

// SignOut clears the session from both the database and the keyring. The
// display name goes too, so the next launch onboards cleanly.
func (s *Service) SignOut(ctx context.Context) error {
	if err := s.store.Delete(secret.KeyPinestemToken); err != nil {
		return err
	}
	if err := s.store.Delete(secret.KeyPinestemPassword); err != nil {
		return err
	}
	if err := s.queries.DeletePinestemAccount(ctx); err != nil {
		return fmt.Errorf("identity: clear account: %w", err)
	}
	if err := s.queries.DeleteProfile(ctx); err != nil {
		return fmt.Errorf("identity: clear profile: %w", err)
	}

	return nil
}

func (s *Service) upsertProfile(ctx context.Context, name, now string) error {
	err := s.queries.UpsertProfile(ctx, sqlc.UpsertProfileParams{
		DisplayName: name,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return fmt.Errorf("identity: persist profile: %w", err)
	}

	return nil
}

func (s *Service) sessionFrom(displayName string, a *pinestem.Account) *Session {
	return &Session{
		DisplayName:      displayName,
		UserName:         a.UserName,
		FirstName:        a.FirstName,
		LastName:         a.LastName,
		CompanyID:        a.CompanyID,
		CompanyName:      a.CompanyName,
		IsProjectManager: a.IsProjectManager,
		IsTeamLead:       a.IsTeamLead,
		SecretsInKeyring: s.keyringOK,
	}
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}

	return 0
}
