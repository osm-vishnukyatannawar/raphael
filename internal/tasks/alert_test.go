package tasks_test

import (
	"strings"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/tasks"
)

func TestAlertTextSingular(t *testing.T) {
	t.Parallel()

	headline, body := tasks.AlertText([]tasks.Task{
		{ShortCode: "REST-2410", Name: "Fix the login redirect loop"},
	})

	if headline != "You have a new task for review" {
		t.Errorf("headline = %q", headline)
	}
	if body != "REST-2410 Fix the login redirect loop" {
		t.Errorf("body = %q", body)
	}
}

func TestAlertTextPlural(t *testing.T) {
	t.Parallel()

	headline, body := tasks.AlertText([]tasks.Task{
		{ShortCode: "REST-2410", Name: "One"},
		{ShortCode: "REST-2411", Name: "Two"},
	})

	if headline != "You have 2 new tasks for review" {
		t.Errorf("headline = %q", headline)
	}
	if strings.Count(body, "\n") != 1 {
		t.Errorf("body = %q, want one task per line", body)
	}
}

// A long list must summarise rather than produce a body the OS will truncate
// mid-word.
func TestAlertTextSummarisesLongLists(t *testing.T) {
	t.Parallel()

	arrived := make([]tasks.Task, 0, 7)
	for i := range 7 {
		arrived = append(arrived, tasks.Task{ShortCode: "REST-" + string(rune('A'+i))})
	}

	headline, body := tasks.AlertText(arrived)

	if headline != "You have 7 new tasks for review" {
		t.Errorf("headline = %q", headline)
	}
	if !strings.HasSuffix(body, "and 4 more") {
		t.Errorf("body = %q, want it to end with a count of the remainder", body)
	}
	if got := strings.Count(body, "\n"); got != 3 {
		t.Errorf("body has %d newlines, want 3 (three named + the summary)", got)
	}
}

func TestAlertTextEmpty(t *testing.T) {
	t.Parallel()

	headline, body := tasks.AlertText(nil)
	if headline != "" || body != "" {
		t.Errorf("got %q/%q, want empty — nothing arrived", headline, body)
	}
}
