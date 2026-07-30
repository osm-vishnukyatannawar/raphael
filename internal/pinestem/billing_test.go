package pinestem_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		Token:            "tok-abc",
		CompanyID:        453,
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
func TestBillingEntriesFollowsPagination(t *testing.T) {
	t.Parallel()

	const (
		total    = 450
		pageSize = 200
	)
	var pagesSeen []float64

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		page, _ := body["PageNumber"].(float64)
		pagesSeen = append(pagesSeen, page)

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
		t.Errorf("got %d entries, want %d", len(entries), total)
	}
	if len(pagesSeen) != 3 || pagesSeen[0] != 1 || pagesSeen[2] != 3 {
		t.Errorf("pages requested = %v, want [1 2 3]", pagesSeen)
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
