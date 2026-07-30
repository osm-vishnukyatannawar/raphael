package pinestem

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// StatusInReview is Pinestem's "3. In review" task status.
//
// The review queue hardcodes it: it is the one status that list is *about*, so
// looking it up would mean matching on a display string that Pinestem lets each
// project rename. Verified against company 453; if a future company numbers
// statuses differently this is the one place to change.
//
// The wider my-tasks list does not hardcode anything — it reads the real set
// from ListTaskStatuses, which is where a company with different numbering is
// handled properly.
const StatusInReview = 4063

// tasksPageLimit matches what the Pinestem web UI requests.
const tasksPageLimit = 100

// Project is an entry from the projects dropdown.
type Project struct {
	ProjectID int64  `json:"projectId"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	StatusID  int64  `json:"statusId"`
}

// Task is a task as Raphael displays it — a subset of a much larger payload.
type Task struct {
	TaskID      int64  `json:"taskId"`
	ShortCode   string `json:"shortCode"`
	Name        string `json:"name"`
	ProjectCode string `json:"projectCode"`
	ProjectName string `json:"projectName"`
	Priority    string `json:"priority"`
	// StatusID is the numeric form of the status; StatusType is its label.
	StatusID       int64  `json:"statusId"`
	StatusType     string `json:"statusType"`
	StatusColor    string `json:"statusColor"`
	DueDate        string `json:"dueDate"`
	ModifiedOn     string `json:"modifiedOn"`
	SprintName     string `json:"sprintName"`
	CompetencyName string `json:"competencyName"`
}

// TaskStatus is one entry from a project's status list.
type TaskStatus struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	// IsDone marks the terminal status. The my-tasks default filter drops it:
	// "everything assigned to me" means open work, not a completed archive.
	IsDone bool `json:"isDone"`
}

type projectsEnvelope struct {
	RecordCount     int           `json:"RecordCount"`
	MultipleResults []wireProject `json:"MultipleResults"`
}

type wireProject struct {
	ProjectID       int64  `json:"ProjectID"`
	ProjectName     string `json:"ProjectName"`
	ProjectCode     string `json:"ProjectCode"`
	ProjectStatusID int64  `json:"ProjectStatusID"`
}

type tasksEnvelope struct {
	RecordCount     int        `json:"RecordCount"`
	MultipleResults []wireTask `json:"MultipleResults"`
}

type wireTask struct {
	TaskID        int64  `json:"TaskID"`
	TaskShortCode string `json:"TaskShortCode"`
	TaskName      string `json:"TaskName"`
	ProjectCode   string `json:"ProjectCode"`
	ProjectName   string `json:"ProjectName"`
	PriorityType  string `json:"PriorityType"`
	// A *string* on the wire, unlike the TaskStatusID query parameter, which is
	// a number. Parsed rather than retyped so a future numeric response still
	// decodes into something usable.
	TaskStatusID   string `json:"TaskStatusID"`
	StatusType     string `json:"StatusType"`
	StatusColor    string `json:"StatusColor"`
	TaskDueDate    string `json:"TaskDueDate"`
	ModifiedOn     string `json:"ModifiedOn"`
	SprintName     string `json:"SprintName"`
	CompetencyName string `json:"CompetencyName"`
}

type statusesEnvelope struct {
	RecordCount     int          `json:"RecordCount"`
	MultipleResults []wireStatus `json:"MultipleResults"`
}

type wireStatus struct {
	ID          int64  `json:"Id"`
	Status      string `json:"Status"`
	StatusColor string `json:"StatusColor"`
	IsDone      bool   `json:"IsDone"`
	IsActive    bool   `json:"IsActive"`
	ProjectID   int64  `json:"ProjectID"`
}

// ActiveProjectStatuses is the narrow filter the task queue uses: the statuses
// the Pinestem task UI itself sends. 37 projects for company 453.
var ActiveProjectStatuses = []int{1, 2}

// AllProjectStatuses is what the billing report UI sends, and what the target
// monitors need — a project can still accrue hours after it leaves the active
// statuses. 80 projects for company 453, against the 37 above.
var AllProjectStatuses = []int{1, 2, 3, 4, 5}

// ListProjects returns the projects the user can read, filtered to the given
// project statuses. Project codes from here feed ListReviewTasks.
func (c *Client) ListProjects(
	ctx context.Context, token string, companyID int64, statuses []int,
) ([]Project, error) {
	q := url.Values{}
	q.Set("ActiveCustomer", "1")
	for _, s := range statuses {
		q.Add("ProjectStatusID", strconv.Itoa(s))
	}
	q.Set("Read", "1")
	q.Set("Write", "0")

	req, err := c.NewAuthenticatedRequest(
		ctx, http.MethodGet, "Projects/ProjectsDropdown?"+q.Encode(), token, companyID, nil,
	)
	if err != nil {
		return nil, err
	}

	var env projectsEnvelope
	if err := c.doJSON(req, &env); err != nil {
		return nil, fmt.Errorf("pinestem: list projects: %w", err)
	}

	projects := make([]Project, 0, len(env.MultipleResults))
	for _, p := range env.MultipleResults {
		projects = append(projects, Project{
			ProjectID: p.ProjectID,
			Code:      p.ProjectCode,
			Name:      p.ProjectName,
			StatusID:  p.ProjectStatusID,
		})
	}

	return projects, nil
}

// ListTaskStatuses returns the distinct task statuses defined across the given
// projects, for the my-tasks status filter.
//
// The endpoint answers per project — a status shared by ten projects comes back
// ten times — so the rows are deduplicated by ID, keeping the first occurrence
// and therefore the server's ordering.
func (c *Client) ListTaskStatuses(
	ctx context.Context, token string, companyID int64, projectCodes []string,
) ([]TaskStatus, error) {
	if len(projectCodes) == 0 {
		return []TaskStatus{}, nil
	}

	q := url.Values{}
	for _, code := range projectCodes {
		q.Add("ProjectCode", code)
	}
	// Status=1 is the *project* status filter this endpoint takes, not a task
	// status — it mirrors what the Pinestem web UI sends.
	q.Set("Status", "1")

	req, err := c.NewAuthenticatedRequest(
		ctx, http.MethodGet, "Projects/ProjectTaskStatuses?"+q.Encode(), token, companyID, nil,
	)
	if err != nil {
		return nil, err
	}

	var env statusesEnvelope
	if err := c.doJSON(req, &env); err != nil {
		return nil, fmt.Errorf("pinestem: list task statuses: %w", err)
	}

	seen := make(map[int64]bool, len(env.MultipleResults))
	statuses := make([]TaskStatus, 0, len(env.MultipleResults))
	for _, s := range env.MultipleResults {
		if seen[s.ID] {
			continue
		}
		seen[s.ID] = true

		statuses = append(statuses, TaskStatus{
			ID:     s.ID,
			Name:   s.Status,
			Color:  s.StatusColor,
			IsDone: s.IsDone,
		})
	}

	return statuses, nil
}

// ListReviewTasks returns the caller's in-review tasks across the given project
// codes, newest-modified first.
//
// userID is the Pinestem UserId from the authentication response. It is
// per-company: the same person has a different UserId in each company they
// belong to, so this must be the ID for companyID.
func (c *Client) ListReviewTasks(
	ctx context.Context, token string, companyID, userID int64, projectCodes []string,
) ([]Task, error) {
	return c.ListAssignedTasks(
		ctx, token, companyID, userID, projectCodes, []int64{StatusInReview},
	)
}

// ListAssignedTasks returns the caller's tasks across the given project codes in
// any of the given statuses, newest-modified first.
//
// Both filters are mandatory in practice: the endpoint treats an omitted
// ProjectCode as "every project" and an omitted TaskStatusID as "every status",
// so passing neither would quietly return the caller's entire history.
func (c *Client) ListAssignedTasks(
	ctx context.Context,
	token string,
	companyID, userID int64,
	projectCodes []string,
	statusIDs []int64,
) ([]Task, error) {
	if len(projectCodes) == 0 || len(statusIDs) == 0 {
		return []Task{}, nil
	}

	var tasks []Task

	for page := 1; ; page++ {
		env, err := c.fetchTaskPage(ctx, token, companyID, userID, projectCodes, statusIDs, page)
		if err != nil {
			return nil, err
		}

		for _, t := range env.MultipleResults {
			statusID, _ := strconv.ParseInt(t.TaskStatusID, 10, 64)

			tasks = append(tasks, Task{
				TaskID:         t.TaskID,
				ShortCode:      t.TaskShortCode,
				Name:           t.TaskName,
				ProjectCode:    t.ProjectCode,
				ProjectName:    t.ProjectName,
				Priority:       t.PriorityType,
				StatusID:       statusID,
				StatusType:     t.StatusType,
				StatusColor:    t.StatusColor,
				DueDate:        normaliseDate(t.TaskDueDate),
				ModifiedOn:     normaliseDate(t.ModifiedOn),
				SprintName:     t.SprintName,
				CompetencyName: t.CompetencyName,
			})
		}

		// Stop on a short page as well as on the count, so a RecordCount that
		// disagrees with reality can't spin this loop forever.
		if len(env.MultipleResults) < tasksPageLimit || len(tasks) >= env.RecordCount {
			break
		}
	}

	if tasks == nil {
		tasks = []Task{}
	}

	return tasks, nil
}

func (c *Client) fetchTaskPage(
	ctx context.Context,
	token string,
	companyID, userID int64,
	projectCodes []string,
	statusIDs []int64,
	page int,
) (*tasksEnvelope, error) {
	q := url.Values{}
	q.Set("AssignedTo", strconv.FormatInt(userID, 10))
	q.Set("ExcludeInformTo", "false")
	q.Set("PageLimit", strconv.Itoa(tasksPageLimit))
	q.Set("PageNumber", strconv.Itoa(page))
	q.Set("Pagination", "true")
	q.Set("SearchTerm", "")
	q.Set("SortingColumn", "ModifiedOn")
	q.Set("SortingOrder", "desc")
	// One repeated parameter per value for both filters, as the web UI sends.
	for _, id := range statusIDs {
		q.Add("TaskStatusID", strconv.FormatInt(id, 10))
	}
	for _, code := range projectCodes {
		q.Add("ProjectCode", code)
	}

	req, err := c.NewAuthenticatedRequest(
		ctx, http.MethodGet, "Tasks/Filter?"+q.Encode(), token, companyID, nil,
	)
	if err != nil {
		return nil, err
	}

	var env tasksEnvelope
	if err := c.doJSON(req, &env); err != nil {
		return nil, fmt.Errorf("pinestem: list tasks (page %d): %w", page, err)
	}

	return &env, nil
}

// doJSON sends a request and decodes a JSON body, treating any non-200 as an error.
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// normaliseDate turns Pinestem's null sentinel into an empty string so callers
// don't have to special-case year 1.
func normaliseDate(s string) string {
	if s == "" || s == "0001-01-01 00:00:00" {
		return ""
	}

	return s
}
