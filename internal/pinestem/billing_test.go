package pinestem_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// Captured live from POST Reports/FilterBillingDetails_New for EmpID 2286.
//
// Two things to notice. BillableHours is *minutes* — 300 renders as "5" in
// BillableHours_HoursFormat. And per-row TotalHours is always 0, so the row
// total has to be computed as billable + non-billable rather than read.
const billingBody = `{
  "RecordCount": 4,
  "MultipleResults": [
    {"ID":1168687,"EmpID":2286,"Date":"2026-07-29 22:20:12","BillableHours":300,
     "NonBillableHours":0,"TotalHours":0,"BillableHours_HoursFormat":"5",
     "TotalHours_HoursFormat":"0","ProjectCode":"RES","ProjectName":"Research and Development",
     "TaskID":"110788","TaskName":"(Component) Billing report","EmpFirstName":"Vishnu"},
    {"ID":1168601,"EmpID":2286,"Date":"2026-07-29 15:44:16","BillableHours":120,
     "NonBillableHours":30,"TotalHours":0,"ProjectCode":"SYS","ProjectName":"Systems",
     "TaskID":"110001","TaskName":"Standup","EmpFirstName":"Vishnu"},
    {"ID":1168542,"EmpID":2286,"Date":"2026-07-28 11:02:00","BillableHours":60,
     "NonBillableHours":0,"TotalHours":0,"ProjectCode":"RES","ProjectName":"Research and Development",
     "TaskID":"110788","TaskName":"(Component) Billing report","EmpFirstName":"Vishnu"},
    {"ID":1168111,"EmpID":2286,"Date":"2026-07-27 09:30:00","BillableHours":30,
     "NonBillableHours":0,"TotalHours":0,"ProjectCode":"RES","ProjectName":"Research and Development",
     "TaskID":"110788","TaskName":"(Component) Billing report","EmpFirstName":"Vishnu"}
  ],
  "ResponseId": 5555, "ErrorMessage": "", "Status": false
}`

func testBillingRequest() pinestem.BillingRequest {
	return pinestem.BillingRequest{
		Token:     "tok-abc",
		CompanyID: 453,
		// CallerID is who is asking, EmpID is whose hours to return. They happen
		// to match for the personal view and diverge for a whole-team monitor.
		CallerID:         2286,
		EmpID:            2286,
		RoleID:           7,
		IsProjectManager: false,
		TimeZone:         "India Standard Time",
		Start:            time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
		End:              time.Date(2026, 7, 30, 23, 59, 59, 0, time.UTC),
	}
}

func TestBillingEntries(t *testing.T) {
	t.Parallel()

	var (
		gotMethod, gotPath, gotToken, gotCompany string
		gotQuery                                 string
		gotBody                                  map[string]any
	)

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotToken = r.Header.Get("AuthenticationToken")
		gotCompany = r.Header.Get("CompanyID")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, billingBody)
	})

	entries, err := client.BillingEntries(t.Context(), testBillingRequest())
	if err != nil {
		t.Fatalf("BillingEntries: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST (the endpoint rejects GET)", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/Reports/FilterBillingDetails_New") {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "isGetAllProjects=true" {
		t.Errorf("query = %q, want isGetAllProjects=true", gotQuery)
	}
	if gotToken != "tok-abc" || gotCompany != "453" {
		t.Errorf("session headers = %q/%q", gotToken, gotCompany)
	}

	// EmpID is the whole point: without it the endpoint returns every employee's
	// hours, which is how the first draft of this feature was wrong.
	if got := gotBody["EmpID"]; got != float64(2286) {
		t.Errorf("EmpID = %v, want 2286", got)
	}
	if got := gotBody["StartDate"]; got != "2026-07-27 00:00:00" {
		t.Errorf("StartDate = %v", got)
	}
	if got := gotBody["EndDate"]; got != "2026-07-30 23:59:59" {
		t.Errorf("EndDate = %v", got)
	}
	// Empty ProjectIds means "every project" — billing needs no project list.
	if got, ok := gotBody["ProjectIds"].([]any); !ok || len(got) != 0 {
		t.Errorf("ProjectIds = %v, want []", gotBody["ProjectIds"])
	}
	if got := gotBody["TimeZone"]; got != "India Standard Time" {
		t.Errorf("TimeZone = %v", got)
	}
	if got := gotBody["RoleID"]; got != float64(7) {
		t.Errorf("RoleID = %v, want 7", got)
	}

	if len(entries) != 4 {
		t.Fatalf("got %d entries, want 4", len(entries))
	}
	if entries[0].BillableMinutes != 300 || entries[0].NonBillableMinutes != 0 {
		t.Errorf("first entry minutes = %+v", entries[0])
	}
	if entries[1].NonBillableMinutes != 30 {
		t.Errorf("non-billable = %d, want 30", entries[1].NonBillableMinutes)
	}
	if entries[0].Date != "2026-07-29 22:20:12" {
		t.Errorf("Date = %q", entries[0].Date)
	}
	if entries[0].ProjectCode != "RES" || entries[0].TaskName != "(Component) Billing report" {
		t.Errorf("metadata = %+v", entries[0])
	}
}

// A wide date range can exceed one page; truncating it would silently
// under-report hours, which is worse than an error.
//
// serverPageCap models the behaviour that matters: Pinestem returns at most 100
// rows however large a PageLimit is requested (50→50, but 100/200/500→100, all
// verified live). An earlier version of this test honoured whatever PageLimit
// the client asked for, which let a client-side limit of 200 look correct while
// really stopping after one short page of 100 and dropping everything after it.
const serverPageCap = 100

func TestBillingEntriesFollowsPagination(t *testing.T) {
	t.Parallel()

	const total = 450
	var pagesSeen []float64
	var requestedLimits []float64

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		page, _ := body["PageNumber"].(float64)
		pagesSeen = append(pagesSeen, page)

		limit, _ := body["PageLimit"].(float64)
		requestedLimits = append(requestedLimits, limit)

		pageSize := min(int(limit), serverPageCap)
		start := (int(page) - 1) * pageSize
		count := min(pageSize, total-start)

		var b strings.Builder
		fmt.Fprintf(&b, `{"RecordCount":%d,"MultipleResults":[`, total)
		for i := range count {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"Date":"2026-07-29 10:00:00","BillableHours":6,"NonBillableHours":0}`)
		}
		b.WriteString(`]}`)

		_, _ = io.WriteString(w, b.String())
	})

	entries, err := client.BillingEntries(t.Context(), testBillingRequest())
	if err != nil {
		t.Fatalf("BillingEntries: %v", err)
	}

	if len(entries) != total {
		t.Errorf("got %d entries, want %d — pages after the first were dropped", len(entries), total)
	}
	// Asking for more than the server will give makes the short-page check fire
	// on a full page, which is exactly how the truncation happened.
	for _, limit := range requestedLimits {
		if limit > serverPageCap {
			t.Errorf("requested PageLimit %.0f, but the server never returns more than %d",
				limit, serverPageCap)

			break
		}
	}
	if len(pagesSeen) != 5 || pagesSeen[0] != 1 || pagesSeen[4] != 5 {
		t.Errorf("pages requested = %v, want [1 2 3 4 5]", pagesSeen)
	}
}

// A day with nothing logged is normal, not an error, and must not decode to nil
// — the caller ranges over the result.
func TestBillingEntriesEmpty(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"RecordCount":0,"MultipleResults":[]}`)
	})

	entries, err := client.BillingEntries(t.Context(), testBillingRequest())
	if err != nil {
		t.Fatalf("BillingEntries: %v", err)
	}
	if entries == nil {
		t.Fatal("entries is nil, want an empty slice")
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestBillingEntriesSurfacesHTTPErrors(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := client.BillingEntries(t.Context(), testBillingRequest()); err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
}

// A monitor asks for every member on a set of projects. Both halves of that are
// easy to get wrong: EmpID must be absent (present-but-zero filters to nobody),
// and isGetAllProjects must disagree with a non-empty ProjectIds or the filter
// is ignored server-side.
func TestBillingEntriesForWholeTeamOnSpecificProjects(t *testing.T) {
	t.Parallel()

	var gotBody map[string]any
	var gotQuery string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = io.WriteString(w, `{"RecordCount":1,"MultipleResults":[
		  {"Date":"2026-07-29 21:32:23","BillableHours":120,"NonBillableHours":0,
		   "EmpID":4001,"EmpFirstName":"Sample Member","ProjectID":773,
		   "ProjectCode":"RAD","ProjectName":"Radiovision","TaskName":"Build"}]}`)
	})

	req := testBillingRequest()
	req.EmpID = 0
	req.ProjectIDs = []int64{773, 782}

	entries, err := client.BillingEntries(t.Context(), req)
	if err != nil {
		t.Fatalf("BillingEntries: %v", err)
	}

	if _, present := gotBody["EmpID"]; present {
		t.Errorf("EmpID = %v was sent; it must be omitted to cover every member", gotBody["EmpID"])
	}
	if gotQuery != "isGetAllProjects=false" {
		t.Errorf("query = %q, want isGetAllProjects=false alongside a project filter", gotQuery)
	}
	ids, _ := gotBody["ProjectIds"].([]any)
	if len(ids) != 2 {
		t.Errorf("ProjectIds = %v, want both", gotBody["ProjectIds"])
	}
	// LoggedInEmpID identifies the asker and must survive EmpID being dropped.
	if gotBody["LoggedInEmpID"] != float64(2286) {
		t.Errorf("LoggedInEmpID = %v, want 2286", gotBody["LoggedInEmpID"])
	}

	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.EmpID != 4001 || e.EmpName != "Sample Member" {
		t.Errorf("member = %d/%q, want 4001/Sample Member", e.EmpID, e.EmpName)
	}
	if e.ProjectID != 773 {
		t.Errorf("ProjectID = %d, want 773 — needed to attribute the row", e.ProjectID)
	}
}

func TestListProjectMembers(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, `{"RecordCount":2,"MultipleResults":[
		  {"ID":4001,"Name":"Sample Member"},{"ID":4002,"Name":"Second Member"}]}`)
	})

	members, err := client.ListProjectMembers(t.Context(), "tok", 453, []string{"RAD", "RES"})
	if err != nil {
		t.Fatalf("ListProjectMembers: %v", err)
	}

	// Repeated ProjectCode returns the union across projects.
	if got := gotQuery["ProjectCode"]; len(got) != 2 || got[0] != "RAD" || got[1] != "RES" {
		t.Errorf("ProjectCode = %v, want [RAD RES]", got)
	}
	if len(members) != 2 || members[0].ID != 4001 {
		t.Errorf("members = %+v", members)
	}
}

// Asking with no codes would return every member in the company, which is never
// what the caller means.
func TestListProjectMembersWithNoCodesSkipsTheCall(t *testing.T) {
	t.Parallel()

	called := false
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = io.WriteString(w, `{"MultipleResults":[]}`)
	})

	members, err := client.ListProjectMembers(t.Context(), "tok", 453, nil)
	if err != nil {
		t.Fatalf("ListProjectMembers: %v", err)
	}
	if called {
		t.Error("the API was called despite there being no project codes")
	}
	if members == nil || len(members) != 0 {
		t.Errorf("members = %+v, want an empty slice", members)
	}
}
