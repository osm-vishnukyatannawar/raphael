package pinestem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// billingPageLimit is how many entries to pull per request.
const billingPageLimit = 200

// billingDateLayout is how Pinestem wants window bounds. Not RFC3339, and no
// timezone: the server compares these against naive stored timestamps, so the
// caller must build them in the account's own zone.
const billingDateLayout = "2006-01-02 15:04:05"

// BillingRequest is everything Reports/FilterBillingDetails_New needs.
//
// EmpID is separate from the session on purpose. Without it the endpoint
// returns *every* employee's hours — verified live, where one project over one
// week came back as the whole team's 56.08h rather than the caller's own. It is
// a parameter rather than "the logged-in user" so that looking up a colleague's
// hours later needs no signature change.
type BillingRequest struct {
	Token            string
	CompanyID        int64
	EmpID            int64
	RoleID           int64
	IsProjectManager bool
	TimeZone         string
	Start            time.Time
	End              time.Time
}

// BillingEntry is one logged block of time.
//
// Minutes, not hours: Pinestem reports integers and 0.1h has no exact binary
// float representation, so rounding stays at the display layer.
type BillingEntry struct {
	Date               string `json:"date"`
	BillableMinutes    int64  `json:"billableMinutes"`
	NonBillableMinutes int64  `json:"nonBillableMinutes"`
	ProjectCode        string `json:"projectCode"`
	ProjectName        string `json:"projectName"`
	TaskName           string `json:"taskName"`
}

// Minutes is the entry's total. The wire's own TotalHours field is always 0 and
// must not be used.
func (e BillingEntry) Minutes() int64 {
	return e.BillableMinutes + e.NonBillableMinutes
}

type billingEnvelope struct {
	RecordCount     int           `json:"RecordCount"`
	MultipleResults []wireBilling `json:"MultipleResults"`
}

// wireBilling names the *Hours fields as Pinestem does even though they carry
// minutes, so the mapping below is the single place that confusion is resolved.
type wireBilling struct {
	Date             string `json:"Date"`
	BillableHours    int64  `json:"BillableHours"`
	NonBillableHours int64  `json:"NonBillableHours"`
	ProjectCode      string `json:"ProjectCode"`
	ProjectName      string `json:"ProjectName"`
	TaskName         string `json:"TaskName"`
}

// BillingEntries returns one row per logged block of time in the window.
//
// Summing these reproduces Reports/GetBillingTotalHours exactly (verified: 2160
// minutes for 2026-07-27..30), so this one call covers both the totals and the
// per-day breakdown and that endpoint is not needed.
func (c *Client) BillingEntries(ctx context.Context, req BillingRequest) ([]BillingEntry, error) {
	entries := []BillingEntry{}

	for page := 1; ; page++ {
		env, err := c.fetchBillingPage(ctx, req, page)
		if err != nil {
			return nil, err
		}

		for _, e := range env.MultipleResults {
			entries = append(entries, BillingEntry{
				Date:               normaliseDate(e.Date),
				BillableMinutes:    e.BillableHours,
				NonBillableMinutes: e.NonBillableHours,
				ProjectCode:        e.ProjectCode,
				ProjectName:        e.ProjectName,
				TaskName:           e.TaskName,
			})
		}

		// Short page as well as count, so a RecordCount that disagrees with
		// reality can't spin this loop forever.
		if len(env.MultipleResults) < billingPageLimit || len(entries) >= env.RecordCount {
			break
		}
	}

	return entries, nil
}

func (c *Client) fetchBillingPage(
	ctx context.Context, req BillingRequest, page int,
) (*billingEnvelope, error) {
	// Field names and casing mirror what the Pinestem web app posts. ProjectIds
	// is empty because an empty list means "all projects".
	body, err := json.Marshal(map[string]any{
		"LoggedInEmpID":    req.EmpID,
		"ProjectManager":   req.EmpID,
		"EmpID":            req.EmpID,
		"CompanyID":        req.CompanyID,
		"RoleID":           req.RoleID,
		"IsProjectManager": req.IsProjectManager,
		"TimeZone":         req.TimeZone,
		"StartDate":        req.Start.Format(billingDateLayout),
		"EndDate":          req.End.Format(billingDateLayout),
		"ProjectIds":       []int64{},
		"PageNumber":       page,
		"PageLimit":        billingPageLimit,
		"Pagination":       true,
		"Sort":             true,
		"SortingColumn":    "Date",
		"SortingOrder":     "desc",
		"IsMobile":         false,
	})
	if err != nil {
		return nil, fmt.Errorf("pinestem: encode billing request: %w", err)
	}

	httpReq, err := c.NewAuthenticatedRequest(
		ctx, http.MethodPost,
		"Reports/FilterBillingDetails_New?isGetAllProjects=true",
		req.Token, req.CompanyID, body,
	)
	if err != nil {
		return nil, err
	}

	var env billingEnvelope
	if err := c.doJSON(httpReq, &env); err != nil {
		return nil, fmt.Errorf("pinestem: billing details (page %d): %w", page, err)
	}

	return &env, nil
}
