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
// Hardcoded because Pinestem exposes no status-lookup endpoint —
// Tasks/TaskStatusDropdown, Masters/TaskStatus, Tasks/StatusDropdown and
// Masters/TaskStatusDropdown all return 404. Verified against company 453; if a
// future company numbers statuses differently this is the one place to change.
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
	TaskID         int64  `json:"taskId"`
	ShortCode      string `json:"shortCode"`
	Name           string `json:"name"`
	ProjectCode    string `json:"projectCode"`
	ProjectName    string `json:"projectName"`
	Priority       string `json:"priority"`
	StatusType     string `json:"statusType"`
	StatusColor    string `json:"statusColor"`
	DueDate        string `json:"dueDate"`
	ModifiedOn     string `json:"modifiedOn"`
	SprintName     string `json:"sprintName"`
	CompetencyName string `json:"competencyName"`
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
	TaskID         int64  `json:"TaskID"`
	TaskShortCode  string `json:"TaskShortCode"`
	TaskName       string `json:"TaskName"`
	ProjectCode    string `json:"ProjectCode"`
	ProjectName    string `json:"ProjectName"`
	PriorityType   string `json:"PriorityType"`
	StatusType     string `json:"StatusType"`
	StatusColor    string `json:"StatusColor"`
	TaskDueDate    string `json:"TaskDueDate"`
	ModifiedOn     string `json:"ModifiedOn"`
	SprintName     string `json:"SprintName"`
	CompetencyName string `json:"CompetencyName"`
}

// ListProjects returns the projects the user can read.
//
// The filter mirrors the Pinestem web UI: active customers, project statuses 1
// and 2, read access. Project codes from here feed ListReviewTasks.
func (c *Client) ListProjects(ctx context.Context, token string, companyID int64) ([]Project, error) {
	q := url.Values{}
	q.Set("ActiveCustomer", "1")
	q.Add("ProjectStatusID", "1")
	q.Add("ProjectStatusID", "2")
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

// ListReviewTasks returns the caller's in-review tasks across the given project
// codes, newest-modified first.
//
// userID is the Pinestem UserId from the authentication response. It is
// per-company: the same person has a different UserId in each company they
// belong to, so this must be the ID for companyID.
func (c *Client) ListReviewTasks(
	ctx context.Context, token string, companyID, userID int64, projectCodes []string,
) ([]Task, error) {
	if len(projectCodes) == 0 {
		// No projects means no tasks; the API would otherwise return every
		// project's tasks, which is emphatically not what the caller asked for.
		return []Task{}, nil
	}

	var tasks []Task

	for page := 1; ; page++ {
		env, err := c.fetchTaskPage(ctx, token, companyID, userID, projectCodes, page)
		if err != nil {
			return nil, err
		}

		for _, t := range env.MultipleResults {
			tasks = append(tasks, Task{
				TaskID:         t.TaskID,
				ShortCode:      t.TaskShortCode,
				Name:           t.TaskName,
				ProjectCode:    t.ProjectCode,
				ProjectName:    t.ProjectName,
				Priority:       t.PriorityType,
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
	ctx context.Context, token string, companyID, userID int64, projectCodes []string, page int,
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
	q.Set("TaskStatusID", strconv.Itoa(StatusInReview))
	// One repeated ProjectCode parameter per project, as the web UI sends.
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
