// Package pinestem is a client for the Pinestem REST API.
//
// Authenticated endpoints take two headers rather than a bearer token:
// AuthenticationToken and CompanyID. Both come from the authentication
// response, which is why the session must persist both values.
package pinestem

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is the production API root. The trailing slash matters —
// paths are appended directly.
const DefaultBaseURL = "https://pinestem.com/api/"

// ErrInvalidCredentials is returned when the API accepts the request but issues
// no token. Callers should surface this as "wrong email or password" rather
// than as a transport failure.
var ErrInvalidCredentials = errors.New("pinestem: invalid credentials")

// Account is a successful authentication, flattened to the fields Raphael uses.
type Account struct {
	UserID           int64  `json:"userId"`
	UserName         string `json:"userName"`
	FirstName        string `json:"firstName"`
	LastName         string `json:"lastName"`
	Token            string `json:"token"`
	CompanyID        int64  `json:"companyId"`
	CompanyName      string `json:"companyName"`
	RoleID           int64  `json:"roleId"`
	IsProjectManager bool   `json:"isProjectManager"`
	IsTeamLead       bool   `json:"isTeamLead"`
	AccountType      string `json:"accountType"`
	TimeZone         string `json:"timeZone"`
	DateTimeFormat   string `json:"dateTimeFormat"`
}

// FullName is the user's name as Pinestem knows it. Used to prefill the display
// name during onboarding.
func (a Account) FullName() string {
	return strings.TrimSpace(a.FirstName + " " + a.LastName)
}

// authEnvelope is the wire shape of POST Users/AuthenticateUser.
//
// Note what is absent: Status and ErrorMessage. A *successful* login returns
// "Status": false with an empty ErrorMessage, so neither field can be used to
// decide whether the call worked. The only reliable signal is a non-empty
// TokenId, which is also what the existing production integrations rely on.
type authEnvelope struct {
	MultipleResults []authResult `json:"MultipleResults"`
}

type authResult struct {
	UserID    int64  `json:"UserId"`
	UserName  string `json:"UserName"`
	FirstName string `json:"FirstName"`
	LastName  string `json:"LastName"`
	TokenID   string `json:"TokenId"`
	// CompanyID is a *string* here ("453") but an integer on every other
	// endpoint, so it is parsed once on the way in.
	CompanyID        string `json:"CompanyID"`
	CompanyName      string `json:"CompanyName"`
	RoleID           int64  `json:"RoleId"`
	IsProjectManager bool   `json:"IsProjectManager"`
	IsTeamLead       bool   `json:"IsTeamLead"`
	AccountType      string `json:"AccountType"`
	TimeZone         string `json:"TimeZone"`
	DateTimeFormat   string `json:"DateTimeFormat"`
}

// Client talks to the Pinestem API. The zero value is not usable; use New.
type Client struct {
	baseURL string
	http    *http.Client
}

// Option customises a Client. Tests use WithBaseURL and WithHTTPClient to point
// at an httptest server.
type Option func(*Client)

func WithBaseURL(u string) Option {
	return func(c *Client) {
		if !strings.HasSuffix(u, "/") {
			u += "/"
		}
		c.baseURL = u
	}
}

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

func New(opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Authenticate exchanges credentials for a long-lived token.
//
// The password is never included in any returned error or logged anywhere; on
// failure callers get ErrInvalidCredentials or a transport error naming only
// the endpoint.
func (c *Client) Authenticate(ctx context.Context, username, password string) (*Account, error) {
	body, err := json.Marshal(map[string]any{
		"UserName":     username,
		"Password":     password,
		"LogAction":    true,
		"IsRememberMe": true,
	})
	if err != nil {
		return nil, fmt.Errorf("pinestem: encode auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"Users/AuthenticateUser", bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("pinestem: build auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// Safe to wrap: transport errors carry method and URL, never the request
		// body, so the password cannot leak into logs through this path.
		return nil, fmt.Errorf("pinestem: authenticate request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pinestem: authenticate returned HTTP %d", resp.StatusCode)
	}

	var env authEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("pinestem: decode auth response: %w", err)
	}

	if len(env.MultipleResults) == 0 || env.MultipleResults[0].TokenID == "" {
		return nil, ErrInvalidCredentials
	}

	return toAccount(env.MultipleResults[0])
}

func toAccount(r authResult) (*Account, error) {
	companyID, err := strconv.ParseInt(strings.TrimSpace(r.CompanyID), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("pinestem: unexpected CompanyID %q: %w", r.CompanyID, err)
	}

	return &Account{
		UserID:           r.UserID,
		UserName:         r.UserName,
		FirstName:        r.FirstName,
		LastName:         r.LastName,
		Token:            r.TokenID,
		CompanyID:        companyID,
		CompanyName:      r.CompanyName,
		RoleID:           r.RoleID,
		IsProjectManager: r.IsProjectManager,
		IsTeamLead:       r.IsTeamLead,
		AccountType:      r.AccountType,
		TimeZone:         r.TimeZone,
		DateTimeFormat:   r.DateTimeFormat,
	}, nil
}

// NewAuthenticatedRequest builds a request carrying the session headers every
// non-auth Pinestem endpoint expects. Feature packages build on this.
func (c *Client) NewAuthenticatedRequest(
	ctx context.Context, method, path, token string, companyID int64, body []byte,
) (*http.Request, error) {
	var rdr *bytes.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	} else {
		rdr = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("pinestem: build %s %s: %w", method, path, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("AuthenticationToken", token)
	req.Header.Set("CompanyID", strconv.FormatInt(companyID, 10))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// Do sends a request built by NewAuthenticatedRequest.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pinestem: request failed: %w", err)
	}

	return resp, nil
}
