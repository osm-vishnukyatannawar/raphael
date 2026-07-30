// Package alerttext formats the body of a desktop notification.
//
// It exists so the two task lists share one rule for how many items to name
// before summarising the rest, rather than each growing its own copy. It knows
// nothing about tasks — callers hand it the lines they want listed.
package alerttext

import (
	"fmt"
	"strings"
)

// ListLimit is how many items to name before summarising the rest.
// Notification bodies get truncated by the OS, and a wall of text is worse than
// a count.
const ListLimit = 3

// Body lists the first ListLimit items, then "and N more" if any are left.
func Body(items []string) string {
	if len(items) == 0 {
		return ""
	}

	lines := make([]string, 0, ListLimit+1)
	lines = append(lines, items[:min(len(items), ListLimit)]...)

	if extra := len(items) - ListLimit; extra > 0 {
		lines = append(lines, fmt.Sprintf("and %d more", extra))
	}

	return strings.Join(lines, "\n")
}
