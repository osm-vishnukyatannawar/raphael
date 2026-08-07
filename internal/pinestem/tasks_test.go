package pinestem_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/pinestem"
)

// Captured live from Projects/ProjectsDropdown.
const projectsBody = `{
  "RecordCount": 3,
  "MultipleResults": [
    {"ProjectID":851,"ProjectName":"Amphenol Website","ProjectCode":"AMP","ProjectStatusID":2,"Read":2,"Write":2,"AIEnabled":true},
    {"ProjectID":870,"ProjectName":"Avows AKKA Training","ProjectCode":"AVO","ProjectStatusID":2,"Read":2,"Write":2,"AIEnabled":true},
    {"ProjectID":782,"ProjectName":"Research and Development","ProjectCode":"RES","ProjectStatusID":2,"Read":2,"Write":2,"AIEnabled":true}
  ],
  "ResponseId": 5555, "ErrorMessage": "", "Status": false
}`

// Captured live from Tasks/Filter. Note TaskStatusID is a *string* and
// EarliestDate uses the year-1 null sentinel.
const tasksBody = `{
  "RecordCount": 2,
  "MultipleResults": [
    {"TaskID":110780,"TaskName":"(Application) HRMS - Issue - UI does not render",
     "TaskShortCode":"REST-2408","TaskDueDate":"2026-07-30 00:00:00",
     "AssignedTo":"Vishnu Kyatannawar","ProjectName":"Research and Development",
     "ProjectCode":"RES","EarliestDate":"0001-01-01 00:00:00","PriorityType":"Medium",
     "StatusType":"3. In review","TaskStatusID":"4063","StatusColor":"#e68fac",
     "SprintName":"2026 - Q3","CompetencyName":"Angular","ModifiedOn":"2026-07-28 23:01:42"},
    {"TaskID":110701,"TaskName":"Fix the login redirect loop",
     "TaskShortCode":"REST-2402","TaskDueDate":"0001-01-01 00:00:00",
     "ProjectName":"Research and Development","ProjectCode":"RES",
     "PriorityType":"High","StatusType":"3. In review","TaskStatusID":"4063",
     "StatusColor":"#e68fac","SprintName":"2026 - Q3","CompetencyName":"Go",
     "ModifiedOn":"2026-07-28 21:56:44"}
  ],
  "ResponseId": 5555, "ErrorMessage": "", "Status": false
}`

func TestListProjects(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values
	var gotToken, gotCompany string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		gotToken = r.Header.Get("AuthenticationToken")
		gotCompany = r.Header.Get("CompanyID")
		_, _ = io.WriteString(w, projectsBody)
	})

	projects, err := client.ListProjects(t.Context(), "tok-abc", 453, pinestem.ActiveProjectStatuses)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}

	if gotToken != "tok-abc" || gotCompany != "453" {
		t.Errorf("session headers = %q/%q", gotToken, gotCompany)
	}
	// ProjectStatusID is sent twice; url.Values must preserve both.
	if got := gotQuery["ProjectStatusID"]; len(got) != 2 || got[0] != "1" || got[1] != "2" {
		t.Errorf("ProjectStatusID = %v, want [1 2]", got)
	}

	if len(projects) != 3 {
		t.Fatalf("got %d projects, want 3", len(projects))
	}
	if projects[2].Code != "RES" || projects[2].Name != "Research and Development" {
		t.Errorf("third project = %+v", projects[2])
	}
}

func TestListReviewTasks(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, tasksBody)
	})

	tasks, err := client.ListReviewTasks(t.Context(), "tok", 453, 2286, []string{"AMP", "AVO", "RES"})
	if err != nil {
		t.Fatalf("ListReviewTasks: %v", err)
	}

	// AssignedTo is the per-company UserId, not a global employee number.
	if got := gotQuery.Get("AssignedTo"); got != "2286" {
		t.Errorf("AssignedTo = %q, want 2286", got)
	}
	// 4063 is the live-verified ID for "3. In review" in company 453. Asserting
	// the literal here (not just the constant) means changing the constant
	// breaks this test rather than silently changing what the app queries.
	if pinestem.StatusInReview != 4063 {
		t.Fatalf("StatusInReview = %d, want 4063", pinestem.StatusInReview)
	}
	if got := gotQuery.Get("TaskStatusID"); got != "4063" {
		t.Errorf("TaskStatusID = %q, want 4063", got)
	}
	if got := gotQuery.Get("SortingColumn"); got != "ModifiedOn" {
		t.Errorf("SortingColumn = %q", got)
	}
	if got := gotQuery.Get("SortingOrder"); got != "desc" {
		t.Errorf("SortingOrder = %q", got)
	}
	// Every project code must go out as its own repeated parameter.
	if got := gotQuery["ProjectCode"]; strings.Join(got, ",") != "AMP,AVO,RES" {
		t.Errorf("ProjectCode = %v, want [AMP AVO RES]", got)
	}

	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}

	// Server order is newest-first and must survive decoding unchanged.
	if tasks[0].ShortCode != "REST-2408" || tasks[1].ShortCode != "REST-2402" {
		t.Errorf("order = %s, %s", tasks[0].ShortCode, tasks[1].ShortCode)
	}
	if tasks[0].ModifiedOn != "2026-07-28 23:01:42" {
		t.Errorf("ModifiedOn = %q", tasks[0].ModifiedOn)
	}
	if tasks[0].DueDate != "2026-07-30 00:00:00" {
		t.Errorf("DueDate = %q", tasks[0].DueDate)
	}
	// The year-1 sentinel must become empty rather than surfacing as a date.
	if tasks[1].DueDate != "" {
		t.Errorf("null due date = %q, want empty", tasks[1].DueDate)
	}
	if tasks[0].CompetencyName != "Angular" || tasks[0].SprintName != "2026 - Q3" {
		t.Errorf("metadata = %+v", tasks[0])
	}
}

// A queue larger than one page must be fully collected, not truncated at 100.
func TestListReviewTasksFollowsPagination(t *testing.T) {
	t.Parallel()

	const total = 250
	var pagesSeen []string

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("PageNumber")
		pagesSeen = append(pagesSeen, page)

		start := 0
		switch page {
		case "1":
			start = 0
		case "2":
			start = 100
		case "3":
			start = 200
		}
		count := min(100, total-start)

		var b strings.Builder
		fmt.Fprintf(&b, `{"RecordCount":%d,"MultipleResults":[`, total)
		for i := range count {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"TaskID":%d,"TaskShortCode":"T-%d","ModifiedOn":"2026-07-28 10:00:00"}`,
				start+i, start+i)
		}
		b.WriteString(`]}`)

		_, _ = io.WriteString(w, b.String())
	})

	tasks, err := client.ListReviewTasks(t.Context(), "tok", 453, 2286, []string{"RES"})
	if err != nil {
		t.Fatalf("ListReviewTasks: %v", err)
	}

	if len(tasks) != total {
		t.Errorf("got %d tasks, want %d", len(tasks), total)
	}
	if strings.Join(pagesSeen, ",") != "1,2,3" {
		t.Errorf("pages requested = %v, want 1,2,3", pagesSeen)
	}
}

// Without project codes the API would return everything; callers expect nothing.
func TestListReviewTasksWithNoProjectsSkipsTheCall(t *testing.T) {
	t.Parallel()

	called := false
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = io.WriteString(w, tasksBody)
	})

	tasks, err := client.ListReviewTasks(t.Context(), "tok", 453, 2286, nil)
	if err != nil {
		t.Fatalf("ListReviewTasks: %v", err)
	}
	if called {
		t.Error("the API was called despite there being no project codes")
	}
	if len(tasks) != 0 {
		t.Errorf("got %d tasks, want 0", len(tasks))
	}
}

func TestListTasksSurfacesHTTPErrors(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := client.ListReviewTasks(t.Context(), "stale", 453, 2286, []string{"RES"}); err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
}

// Captured from Projects/ProjectTaskStatuses, trimmed. The endpoint returns a
// row per project, so the same status appears once per project that defines it —
// note 1824 twice, for projects 851 and 782.
const statusesBody = `{
  "RecordCount": 4,
  "MultipleResults": [
    {"Id":1824,"Status":"1. Planned","ProjectID":851,"StatusColor":"#848482","IsDone":false,"IsActive":true},
    {"Id":1824,"Status":"1. Planned","ProjectID":782,"StatusColor":"#848482","IsDone":false,"IsActive":true},
    {"Id":4063,"Status":"3. In review","ProjectID":851,"StatusColor":"#e68fac","IsDone":false,"IsActive":true},
    {"Id":1823,"Status":"9. Done","ProjectID":851,"StatusColor":"#0067a5","IsDone":true,"IsActive":true}
  ],
  "ResponseId": 5555, "ErrorMessage": "", "Status": false
}`

func TestListTaskStatuses(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, statusesBody)
	})

	statuses, err := client.ListTaskStatuses(t.Context(), "tok", 453, []string{"AMP", "RES"})
	if err != nil {
		t.Fatalf("ListTaskStatuses: %v", err)
	}

	if got := gotQuery["ProjectCode"]; strings.Join(got, ",") != "AMP,RES" {
		t.Errorf("ProjectCode = %v, want [AMP RES]", got)
	}
	if got := gotQuery.Get("Status"); got != "1" {
		t.Errorf("Status = %q, want 1", got)
	}

	// The duplicate 1824 must collapse: this list drives a checkbox picker, and
	// the same status listed once per project would render it several times.
	if len(statuses) != 3 {
		t.Fatalf("got %d statuses, want 3 (deduped): %+v", len(statuses), statuses)
	}
	if statuses[0].ID != 1824 || statuses[0].Name != "1. Planned" {
		t.Errorf("first status = %+v", statuses[0])
	}
	if statuses[0].Color != "#848482" {
		t.Errorf("Color = %q", statuses[0].Color)
	}
	if statuses[2].ID != 1823 || !statuses[2].IsDone {
		t.Errorf("done status = %+v, want 1823 with IsDone", statuses[2])
	}
	if statuses[1].IsDone {
		t.Errorf("%+v should not be marked done", statuses[1])
	}
}

func TestListTaskStatusesWithNoProjectsSkipsTheCall(t *testing.T) {
	t.Parallel()

	called := false
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = io.WriteString(w, statusesBody)
	})

	statuses, err := client.ListTaskStatuses(t.Context(), "tok", 453, nil)
	if err != nil {
		t.Fatalf("ListTaskStatuses: %v", err)
	}
	if called {
		t.Error("the API was called despite there being no project codes")
	}
	if len(statuses) != 0 {
		t.Errorf("got %d statuses, want 0", len(statuses))
	}
}

// The my-tasks list asks for several statuses at once, which the API takes as a
// repeated parameter exactly like ProjectCode.
func TestListAssignedTasksSendsEveryStatus(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, tasksBody)
	})

	tasks, err := client.ListAssignedTasks(
		t.Context(), "tok", 453, 2286, []string{"RES"}, []int64{1824, 1825, 4063},
	)
	if err != nil {
		t.Fatalf("ListAssignedTasks: %v", err)
	}

	if got := gotQuery["TaskStatusID"]; strings.Join(got, ",") != "1824,1825,4063" {
		t.Errorf("TaskStatusID = %v, want [1824 1825 4063]", got)
	}
	if got := gotQuery.Get("AssignedTo"); got != "2286" {
		t.Errorf("AssignedTo = %q, want 2286", got)
	}

	// TaskStatusID comes back as a *string* on the task rows; it has to be
	// usable as a number so the UI can group by status.
	if len(tasks) != 2 || tasks[0].StatusID != 4063 {
		t.Errorf("StatusID = %d, want 4063", tasks[0].StatusID)
	}
}

// No statuses means no filter at all, which would return every status. The
// caller asked for a specific set, so an empty one is nothing.
func TestListAssignedTasksWithNoStatusesSkipsTheCall(t *testing.T) {
	t.Parallel()

	called := false
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		_, _ = io.WriteString(w, tasksBody)
	})

	tasks, err := client.ListAssignedTasks(t.Context(), "tok", 453, 2286, []string{"RES"}, nil)
	if err != nil {
		t.Fatalf("ListAssignedTasks: %v", err)
	}
	if called {
		t.Error("the API was called despite there being no statuses")
	}
	if len(tasks) != 0 {
		t.Errorf("got %d tasks, want 0", len(tasks))
	}
}

// A team board asks about someone else, and must not be handed tasks the member
// is merely informed on. Verified live: with ExcludeInformTo=false, asking for
// one member returns rows owned by other people.
func TestListTasksAssignedToExcludesInformedOn(t *testing.T) {
	t.Parallel()

	var gotQuery url.Values

	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = io.WriteString(w, tasksBody)
	})

	_, err := client.ListTasksAssignedTo(
		t.Context(), "tok", 453, 4001, []string{"RES"}, []int64{4063}, true,
	)
	if err != nil {
		t.Fatalf("ListTasksAssignedTo: %v", err)
	}

	if got := gotQuery.Get("ExcludeInformTo"); got != "true" {
		t.Errorf("ExcludeInformTo = %q, want true", got)
	}
	// One member per call. Repeated AssignedTo returns zero rows server-side and
	// a comma-joined list returns rows with EmpID 0, so this must stay single.
	if got := gotQuery["AssignedTo"]; len(got) != 1 || got[0] != "4001" {
		t.Errorf("AssignedTo = %v, want [4001]", got)
	}
}

// The existing callers ask about the signed-in user, where a task they are
// informed on is still theirs to see. Widening the client must not change that.
func TestExistingCallersStillIncludeInformedOn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		call func(*pinestem.Client) error
	}{
		{"ListReviewTasks", func(c *pinestem.Client) error {
			_, err := c.ListReviewTasks(t.Context(), "tok", 453, 2286, []string{"RES"})

			return err
		}},
		{"ListAssignedTasks", func(c *pinestem.Client) error {
			_, err := c.ListAssignedTasks(t.Context(), "tok", 453, 2286, []string{"RES"}, []int64{4063})

			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotQuery url.Values

			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				_, _ = io.WriteString(w, tasksBody)
			})

			if err := tc.call(client); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got := gotQuery.Get("ExcludeInformTo"); got != "false" {
				t.Errorf("ExcludeInformTo = %q, want false", got)
			}
		})
	}
}

// A team board buckets rows by member, so it has to know who a row belongs to
// rather than trusting that the answer matches the question.
func TestListTasksAssignedToMapsTheAssignee(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"RecordCount":1,"MultipleResults":[
		  {"TaskID":1,"TaskName":"Sample","TaskShortCode":"RES-1","ProjectCode":"RES",
		   "ProjectName":"Research","TaskStatusID":"4063","AssignedToEmpID":4001,
		   "ModifiedOn":"2026-07-28 21:56:44","TaskDueDate":"0001-01-01 00:00:00"}]}`)
	})

	tasks, err := client.ListTasksAssignedTo(
		t.Context(), "tok", 453, 4001, []string{"RES"}, []int64{4063}, true,
	)
	if err != nil {
		t.Fatalf("ListTasksAssignedTo: %v", err)
	}
	if len(tasks) != 1 || tasks[0].AssignedToEmpID != 4001 {
		t.Errorf("tasks = %+v, want AssignedToEmpID 4001", tasks)
	}
	// The year-1 sentinel must not reach the cache as a sortable date.
	if tasks[0].DueDate != "" {
		t.Errorf("DueDate = %q, want empty", tasks[0].DueDate)
	}
}
