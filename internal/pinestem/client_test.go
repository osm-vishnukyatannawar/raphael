package pinestem_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// Trimmed from a real successful response. Note "Status": false and an empty
// ErrorMessage on a login that *worked* — the reason none of those fields can
// be used to detect success.
const successBody = `{
  "RecordCount": 1,
  "MultipleResults": [{
    "UserId": 6406,
    "FirstName": "Venkata Krishna Dinesh",
    "LastName": "Madireddy",
    "TokenId": "00f04112-ec84-4c97-bd89-8afa7c63368d",
    "IsProjectManager": false,
    "RoleId": 2294,
    "UserName": "someone@example.com",
    "CompanyID": "453",
    "IsTeamLead": false,
    "DateTimeFormat": "yyyy-MM-dd HH:mm:ss",
    "AccountType": "premium",
    "TimeZone": "India Standard Time",
    "CompanyName": "Osmosys"
  }],
  "ResponseId": 5555,
  "ErrorMessage": "",
  "Status": false
}`

func newTestClient(t *testing.T, handler http.HandlerFunc) *pinestem.Client {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return pinestem.New(
		pinestem.WithBaseURL(srv.URL),
		pinestem.WithHTTPClient(srv.Client()),
	)
}

func TestAuthenticateSuccess(t *testing.T) {
	t.Parallel()

	var gotPath, gotContentType string
	var gotBody map[string]any

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, successBody)
	})

	acct, err := client.Authenticate(t.Context(), "someone@example.com", "hunter2")
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	if gotPath != "/Users/AuthenticateUser" {
		t.Errorf("path = %q, want /Users/AuthenticateUser", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["UserName"] != "someone@example.com" || gotBody["Password"] != "hunter2" {
		t.Errorf("credentials not sent as expected: %v", gotBody)
	}

	if acct.Token != "00f04112-ec84-4c97-bd89-8afa7c63368d" {
		t.Errorf("Token = %q", acct.Token)
	}
	// The headline decode risk: CompanyID is a JSON string but must land as an int.
	if acct.CompanyID != 453 {
		t.Errorf("CompanyID = %d, want 453", acct.CompanyID)
	}
	if acct.CompanyName != "Osmosys" {
		t.Errorf("CompanyName = %q, want Osmosys", acct.CompanyName)
	}
	if got, want := acct.FullName(), "Venkata Krishna Dinesh Madireddy"; got != want {
		t.Errorf("FullName() = %q, want %q", got, want)
	}
}

// The trap: "Status": false accompanies a perfectly good token. Anything that
// keys off Status would reject a valid login.
func TestAuthenticateIgnoresStatusFalse(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{
          "MultipleResults": [{"TokenId": "tok-123", "CompanyID": "7", "UserId": 1}],
          "Status": false,
          "ErrorMessage": ""
        }`)
	})

	acct, err := client.Authenticate(t.Context(), "u", "p")
	if err != nil {
		t.Fatalf("Status:false must not be treated as failure, got: %v", err)
	}
	if acct.Token != "tok-123" {
		t.Errorf("Token = %q, want tok-123", acct.Token)
	}
}

func TestAuthenticateRejectsMissingToken(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		// Captured live from Pinestem with deliberately bogus credentials. Note
		// what it does NOT give you: the HTTP status is 200, Status is false
		// exactly as on success, and ErrorMessage is empty. MultipleResults is
		// absent rather than null or empty. The missing token is the only signal.
		"real rejection": `{"RecordCount":0,"ResponseId":-1,"ErrorMessage":"",` +
			`"DetailedErrorMessage":"","Status":false}`,
		"empty results": `{"MultipleResults": [], "ErrorMessage": "Invalid credentials"}`,
		"null results":  `{"MultipleResults": null}`,
		"blank token":   `{"MultipleResults": [{"TokenId": "", "CompanyID": "453"}]}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, body)
			})

			_, err := client.Authenticate(t.Context(), "u", "wrong")
			if !errors.Is(err, pinestem.ErrInvalidCredentials) {
				t.Errorf("err = %v, want ErrInvalidCredentials", err)
			}
		})
	}
}

// A password must never reach an error string that could be logged.
func TestAuthenticateErrorsOmitPassword(t *testing.T) {
	t.Parallel()

	const password = "sup3r-s3cret-passphrase"

	cases := map[string]http.HandlerFunc{
		"http 500": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"bad json": func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "not json") },
		"bad company": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"MultipleResults":[{"TokenId":"t","CompanyID":"abc"}]}`)
		},
		"no token": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"MultipleResults":[]}`)
		},
	}

	for name, handler := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, handler)

			_, err := client.Authenticate(t.Context(), "user@example.com", password)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), password) {
				t.Errorf("password leaked into error: %v", err)
			}
		})
	}
}

func TestAuthenticateHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = io.WriteString(w, successBody)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	if _, err := client.Authenticate(ctx, "u", "p"); err == nil {
		t.Fatal("expected a timeout error")
	}
}

func TestNewAuthenticatedRequestSetsSessionHeaders(t *testing.T) {
	t.Parallel()

	client := pinestem.New(pinestem.WithBaseURL("https://example.test/api"))

	req, err := client.NewAuthenticatedRequest(
		t.Context(), http.MethodGet, "Users/Companies", "tok-abc", 453, nil,
	)
	if err != nil {
		t.Fatalf("NewAuthenticatedRequest: %v", err)
	}

	if got := req.Header.Get("AuthenticationToken"); got != "tok-abc" {
		t.Errorf("AuthenticationToken = %q", got)
	}
	// Sent as a header string even though it is an integer internally.
	if got := req.Header.Get("CompanyID"); got != "453" {
		t.Errorf("CompanyID = %q, want 453", got)
	}
	if got := req.URL.String(); got != "https://example.test/api/Users/Companies" {
		t.Errorf("url = %q", got)
	}
}
