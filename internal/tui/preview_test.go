package tui

import (
	"strings"

	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/render"
)

// previewChips and previewRest are the review the confirmation fixtures show: a chips row, the opening slot beneath
// it as render.Body places it, and one finding. Each part carries text the display has to make safe.
const (
	previewChips = "`\u26d4 1 blocking`"
	previewRest  = "> [!NOTE]\n\nSummary \u202eevil\n\n<details>\n<summary>Title</summary>\n\nBody \x1b[31m\n\n</details>\n"
)

func confirmPreview() publish.Preview {
	p := publish.Preview{
		Comments:     []publish.Comment{{Path: "a.go", Line: 12, Side: "RIGHT", StartLine: 10, StartSide: "RIGHT", Body: "**Inline** \u2066body"}},
		EnvelopeJSON: "{\n  \"event\": \"COMMENT\",\n  \"body\": \"x\\u202e\"\n}",
	}
	p.Compose = composeOpening(p, nil)
	env, _, err := p.Compose("")
	if err != nil {
		panic(err)
	}
	p.Body = env.Body
	return p
}

// composeOpening stands in for publish.Run's closure: it puts the message where render.Body puts the opening prose,
// under the chips row. With refuse set it refuses a message carrying a raw tag, as the allowlist does.
func composeOpening(base publish.Preview, refuse error) func(string) (publish.Envelope, string, error) {
	return func(message string) (publish.Envelope, string, error) {
		if refuse != nil && strings.Contains(message, "<") {
			return publish.Envelope{}, "", refuse
		}
		head := previewChips
		if message != "" {
			head += "\n\n" + message
		}
		return publish.Envelope{Body: head + "\n\n---\n\n" + previewRest, Comments: base.Comments}, base.EnvelopeJSON, nil
	}
}

// movedPreview has more commits than it lists and a commit subject carrying an escape sequence.
func movedPreview() publish.Preview {
	p := confirmPreview()
	p.HeadMoved = &publish.HeadMoved{Captured: "1111111111111111111111111111111111111111", Live: "4444444444444444444444444444444444444444", AheadBy: 23,
		Commits: []publish.MovedCommit{{SHA: "0100000", Subject: "fix \x1b[31mred"}, {SHA: "0200000", Subject: "second"}},
		Touched: []string{"f-001", "f-002"}}
	return p
}

// movedHeadLines is what the confirmation must show for movedPreview.
var movedHeadLines = []string{"Head moved 23 commits since capture (1111111 to 4444444)", "pinned to the captured commit 1111111",
	"GitHub will not mark its comments outdated for these commits.", "A comment on a line they changed shows beside the new code.", "0100000 fix \\u001B[31mred", "0200000 second", "and 21 earlier",
	"Findings on changed files: f-001, f-002"}

// pillPreview is a confirmation fixture whose body row and inline comment each carry a real severity pill, and whose
// Compose puts a message above the row as render.Body does.
func pillPreview() publish.Preview {
	c := render.Comments(render.Input{Inline: "all", Findings: []render.Finding{{ID: "f-001", Title: "T", Body: "B.",
		Label: "issue", Severity: "major", Location: &render.Location{Path: "a.go", Side: "RIGHT", Line: 3}}}})
	row, _, _ := strings.Cut(c[0].Body, "\n")
	p := confirmPreview()
	p.Comments = []publish.Comment{{Path: "a.go", Line: 3, Side: "RIGHT", Body: c[0].Body}}
	p.Compose = func(message string) (publish.Envelope, string, error) {
		body := "<details>\n<summary>" + row + "</summary>\n\nB.\n\n</details>\n"
		if message != "" {
			body = message + "\n\n" + body
		}
		return publish.Envelope{Body: body, Comments: p.Comments}, p.EnvelopeJSON, nil
	}
	env, _, _ := p.Compose("")
	p.Body = env.Body
	return p
}

// recordPreview is a confirmation fixture whose body is render.Body's, record line included.
func recordPreview() publish.Preview {
	in := render.Input{Owner: "acme", Repo: "widgets", Number: 42, Round: 1, HeadSHA: "abc1234", Inline: "none",
		Digest: "d", PublicationID: "p", Findings: []render.Finding{{ID: "f-001", Title: "T", Body: "B.", General: true, Label: "issue"}}}
	p := confirmPreview()
	p.Comments = nil
	p.Compose = func(message string) (publish.Envelope, string, error) {
		composed := in
		composed.Summary = message
		return publish.Envelope{Body: render.Body(composed)}, p.EnvelopeJSON, nil
	}
	env, _, _ := p.Compose("")
	p.Body = env.Body
	return p
}

// editedURL is the review a sticky confirmation edits.
const editedURL = "https://github.com/acme/widgets/pull/42#pullrequestreview-77"
