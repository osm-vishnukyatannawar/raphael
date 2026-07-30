package mytasks

import (
	"fmt"
	"strings"

	"github.com/osm-vishnukyatannawar/raphael/internal/alerttext"
)

// AlertText is the wording for a newly assigned task.
//
// Worded differently from the review queue's on purpose: both alerts can land on
// the same desktop, and "assigned to you" versus "for review" is the only thing
// distinguishing them once the notification is on screen.
func AlertText(arrived []Task) (headline, body string) {
	if len(arrived) == 0 {
		return "", ""
	}

	if len(arrived) == 1 {
		headline = "A new task is assigned to you"
	} else {
		headline = fmt.Sprintf("%d new tasks are assigned to you", len(arrived))
	}

	items := make([]string, 0, len(arrived))
	for _, t := range arrived {
		items = append(items, strings.TrimSpace(t.ShortCode+" "+t.Name))
	}

	return headline, alerttext.Body(items)
}
