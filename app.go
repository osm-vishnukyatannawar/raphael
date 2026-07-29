package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"runtime"

	"github.com/osm-vishnukyatannawar/raphael/internal/db"
	"github.com/osm-vishnukyatannawar/raphael/internal/identity"
	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
	"github.com/osm-vishnukyatannawar/raphael/internal/secret"
)

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

	a.identity = identity.New(database, pinestem.New(), store, keyringOK)
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
