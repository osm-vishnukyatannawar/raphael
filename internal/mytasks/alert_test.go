package mytasks_test

import (
	"strings"
	"testing"

	"github.com/osm-vishnukyatannawar/raphael/internal/mytasks"
)

func TestAlertText(t *testing.T) {
	t.Parallel()

	arrived := []mytasks.Task{
		{ShortCode: "REST-2411", Name: "New assignment"},
		{ShortCode: "AMP-118", Name: "Landing page copy"},
		{ShortCode: "AMP-119", Name: "Footer links"},
		{ShortCode: "AMP-120", Name: "Contact form"},
	}

	headline, body := mytasks.AlertText(arrived[:1])
	// Distinct from the review queue's wording: both alerts can land on the same
	// desktop, and the headline is all that separates them.
	if headline != "A new task is assigned to you" {
		t.Errorf("headline = %q", headline)
	}
	if body != "REST-2411 New assignment" {
		t.Errorf("body = %q", body)
	}

	headline, body = mytasks.AlertText(arrived)
	if headline != "4 new tasks are assigned to you" {
		t.Errorf("headline = %q", headline)
	}
	// Three named, the rest counted.
	if lines := strings.Split(body, "\n"); len(lines) != 4 || lines[3] != "and 1 more" {
		t.Errorf("body = %q", body)
	}
	if strings.Contains(body, "AMP-120") {
		t.Errorf("the fourth task was named rather than counted: %q", body)
	}
}

func TestAlertTextIsEmptyWithNothingNew(t *testing.T) {
	t.Parallel()

	headline, body := mytasks.AlertText(nil)
	if headline != "" || body != "" {
		t.Errorf("got %q/%q, want empty", headline, body)
	}
}
