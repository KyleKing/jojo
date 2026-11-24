package stack

import (
	"strconv"
	"strings"
)

// The markers a managed section sits between. Everything outside them is the author's and is never
// rewritten; everything between them is jojo's and is replaced whole on every submit.
const (
	SectionStart = "<!-- jojo:stack -->"
	SectionEnd   = "<!-- /jojo:stack -->"
)

func itoa(n int) string { return strconv.Itoa(n) }

// Section is the stack's navigation for step i's pull request: every pull request of the stack, top
// first the way a reviewer reads a stack, with this one marked and trunk at the bottom.
func Section(plan Plan, i int, numbers map[string]int) string {
	lines := []string{
		SectionStart, "**Stack** (top first, managed by jojo; edits between the markers are replaced)", "",
	}

	for j := len(plan.Steps) - 1; j >= 0; j-- {
		s := &plan.Steps[j]

		ref := "`" + s.Bookmark + "`"
		if n, ok := numbers[s.Bookmark]; ok && n > 0 {
			ref = "#" + itoa(n)
		}

		line := "- " + ref + " " + s.Change.Subject
		if s.Action == Merged {
			line += " (merged)"
		}

		if j == i {
			line = "- **" + strings.TrimPrefix(line, "- ") + "** ← this pull request"
		}

		lines = append(lines, line)
	}

	lines = append(lines, "- `"+plan.Trunk+"`", SectionEnd)

	return strings.Join(lines, "\n")
}

// Compose puts section into body: in place of the one already there, or after the author's text.
func Compose(body, section string) string {
	start := strings.Index(body, SectionStart)
	end := strings.Index(body, SectionEnd)

	if start >= 0 && end > start {
		return body[:start] + section + body[end+len(SectionEnd):]
	}

	body = strings.TrimRight(body, "\n ")
	if body == "" {
		return section
	}

	return body + "\n\n" + section
}

// normalized is a body as GitHub may hand it back: line endings a browser edit turned to CRLF, and
// the trailing newline gh drops, are not a difference worth an edit.
func normalized(body string) string {
	return strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
}

// Body is step i's pull request body once a submit has run: the author's text, which for a new pull
// request is the change's description after its subject, with the stack section in place.
func Body(plan Plan, i int, numbers map[string]int) string {
	s := &plan.Steps[i]

	text := strings.TrimSpace(s.Change.Body)
	if s.PR != nil && s.Action != Create {
		text = s.PR.Body
	}

	return Compose(text, Section(plan, i, numbers))
}
