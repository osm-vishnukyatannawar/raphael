package tasks

import (
	"fmt"
	"strings"

	"github.com/osm-vishnukyatannawar/raphael/internal/alerttext"
)

// AlertText is the wording for a new-task notification: the phrase Raphael
// leads with, and the list of what arrived.
//
// Kept here as a pure function so the wording is testable without a desktop
// session, and so internal/notify stays free of any knowledge of tasks. The
// truncation rule lives in internal/alerttext, shared with the assigned-task
// list.
func AlertText(arrived []Task) (headline, body string) {
	if len(arrived) == 0 {
		return "", ""
	}

	if len(arrived) == 1 {
		headline = "You have a new task for review"
	} else {
		headline = fmt.Sprintf("You have %d new tasks for review", len(arrived))
	}

	items := make([]string, 0, len(arrived))
	for _, t := range arrived {
		items = append(items, strings.TrimSpace(t.ShortCode+" "+t.Name))
	}

	return headline, alerttext.Body(items)
}
