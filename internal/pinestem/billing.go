package pinestem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// billingPageLimit is how many entries to pull per request.
//
// 100 is the server's own cap, not a preference: asking for 50 returns 50, but
// 100, 200 and 500 all return exactly 100. Setting this any higher breaks the
// short-page check below — a full 100-row page reads as "shorter than the limit,
// so this is the last one" and every later page is silently dropped. That is a
// wrong total rather than an error, so keep this equal to the real cap.
const billingPageLimit = 100

// billingDateLayout is how Pinestem wants window bounds. Not RFC3339, and no
// timezone: the server compares these against naive stored timestamps, so the
// caller must build them in the account's own zone.
const billingDateLayout = "2006-01-02 15:04:05"

// BillingRequest is everything Reports/FilterBillingDetails_New needs.
//
// EmpID and CallerID are separate because the endpoint uses them for different
// things, and conflating them is how the personal figures were wrong at first.
// CallerID identifies who is asking; EmpID filters whose hours come back.
//
// Leaving EmpID at zero omits the filter, which returns *every* member's rows —
// that is what the whole-team monitors rely on, and equally what made a single
// project over one week report the team's 56.08h instead of the caller's own.
// Both behaviours are wanted; which one applies must be explicit.
type BillingRequest struct {
	Token     string
	CompanyID int64
	// CallerID is the signed-in user (LoggedInEmpID on the wire).
	CallerID int64
	// EmpID filters to one member. Zero means every member.
	EmpID int64
	// ProjectIDs filters to specific projects. Empty means every project.
	ProjectIDs       []int64
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
	EmpID              int64  `json:"empId"`
	EmpName            string `json:"empName"`
	ProjectID          int64  `json:"projectId"`
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
	EmpID            int64  `json:"EmpID"`
	// EmpFirstName carries the member's *full* display name, not just the
	// first name; EmpLastName is always null on this endpoint.
	EmpFirstName string `json:"EmpFirstName"`
	ProjectID    int64  `json:"ProjectID"`
	ProjectCode  string `json:"ProjectCode"`
	ProjectName  string `json:"ProjectName"`
	TaskName     string `json:"TaskName"`
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
				EmpID:              e.EmpID,
				EmpName:            strings.TrimSpace(e.EmpFirstName),
				ProjectID:          e.ProjectID,
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
	projectIDs := req.ProjectIDs
	if projectIDs == nil {
		projectIDs = []int64{}
	}

	// Field names and casing mirror what the Pinestem web app posts.
	payload := map[string]any{
		"LoggedInEmpID":    req.CallerID,
		"ProjectManager":   req.CallerID,
		"CompanyID":        req.CompanyID,
		"RoleID":           req.RoleID,
		"IsProjectManager": req.IsProjectManager,
		"TimeZone":         req.TimeZone,
		"StartDate":        req.Start.Format(billingDateLayout),
		"EndDate":          req.End.Format(billingDateLayout),
		"ProjectIds":       projectIDs,
		"PageNumber":       page,
		"PageLimit":        billingPageLimit,
		"Pagination":       true,
		"Sort":             true,
		"SortingColumn":    "Date",
		"SortingOrder":     "desc",
		"IsMobile":         false,
	}
	// Omitted entirely rather than sent as 0: the filter is applied whenever the
	// key is present, and 0 matches nobody.
	if req.EmpID != 0 {
		payload["EmpID"] = req.EmpID
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("pinestem: encode billing request: %w", err)
	}

	// isGetAllProjects has to agree with ProjectIds or the filter is ignored.
	path := "Reports/FilterBillingDetails_New?isGetAllProjects=" +
		strconv.FormatBool(len(projectIDs) == 0)

	httpReq, err := c.NewAuthenticatedRequest(
		ctx, http.MethodPost, path, req.Token, req.CompanyID, body,
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

// Member is an entry from the project members dropdown.
//
// ID is the same identifier the billing rows call EmpID, which is what makes it
// usable directly as a target's subject.
type Member struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type membersEnvelope struct {
	MultipleResults []Member `json:"MultipleResults"`
}

// ListProjectMembers returns the union of members across the given project
// codes, sorted by name as the API returns them.
//
// Passing no codes asks for every member in the company, which is rarely what a
// caller wants, so it returns empty instead — the same guard ListReviewTasks
// uses for the equivalent mistake.
func (c *Client) ListProjectMembers(
	ctx context.Context, token string, companyID int64, projectCodes []string,
) ([]Member, error) {
	if len(projectCodes) == 0 {
		return []Member{}, nil
	}

	q := url.Values{}
	for _, code := range projectCodes {
		q.Add("ProjectCode", code)
	}

	req, err := c.NewAuthenticatedRequest(
		ctx, http.MethodGet, "Projects/ProjectMembersDropdown?"+q.Encode(),
		token, companyID, nil,
	)
	if err != nil {
		return nil, err
	}

	var env membersEnvelope
	if err := c.doJSON(req, &env); err != nil {
		return nil, fmt.Errorf("pinestem: list project members: %w", err)
	}

	if env.MultipleResults == nil {
		return []Member{}, nil
	}

	return env.MultipleResults, nil
}
