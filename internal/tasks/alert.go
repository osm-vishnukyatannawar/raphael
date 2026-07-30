package tasks

import (
	"fmt"
	"strings"
)

// alertListLimit is how many tasks to name before summarising the rest. Notification
// bodies get truncated by the OS, and a wall of text is worse than a count.
const alertListLimit = 3

// AlertText is the wording for a new-task notification: the phrase Raphael
// leads with, and the list of what arrived.
//
// Kept here as a pure function so the wording is testable without a desktop
// session, and so internal/notify stays free of any knowledge of tasks.
func AlertText(arrived []Task) (headline, body string) {
	if len(arrived) == 0 {
		return "", ""
	}

	if len(arrived) == 1 {
		headline = "You have a new task for review"
	} else {
		headline = fmt.Sprintf("You have %d new tasks for review", len(arrived))
	}

	lines := make([]string, 0, alertListLimit+1)
	for _, t := range arrived[:min(len(arrived), alertListLimit)] {
		lines = append(lines, strings.TrimSpace(t.ShortCode+" "+t.Name))
	}
	if extra := len(arrived) - alertListLimit; extra > 0 {
		lines = append(lines, fmt.Sprintf("and %d more", extra))
	}

	return headline, strings.Join(lines, "\n")
}
