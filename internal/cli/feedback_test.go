package cli

import (
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
)

func TestFeedbackJSON(t *testing.T) {
	home, _ := sendBackRun(t)
	if code, env, _ := execIn(t, home, "", "reply", "n-001", "--body", "Added."); code != 0 {
		t.Fatalf("reply exit %d envelope %v", code, env)
	}
	code, env, _ := execIn(t, home, "", "feedback")
	if code != 0 || env["version"] != float64(1) {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	readiness, _ := env["readiness"].(map[string]any)
	notes, _ := env["notes"].([]any)
	findings, _ := env["findings"].([]any)
	if readiness["ready"] != false || len(notes) != 1 || len(findings) != 2 {
		t.Fatalf("envelope %v", env)
	}
	note, _ := notes[0].(map[string]any)
	replies, _ := note["replies"].([]any)
	if note["id"] != "n-001" || note["findingId"] != "f-002" || note["status"] != "open" || note["body"] != "Needs evidence." || note["at"] == nil || len(replies) != 1 {
		t.Fatalf("note %v", note)
	}
	f, _ := findings[0].(map[string]any)
	if f["id"] != "f-001" || f["title"] != "One" || f["disposition"] != "accepted" || f["rev"] != float64(1) || len(f) != 4 {
		t.Fatalf("finding %v", f)
	}
}

func TestPrintFeedbackEscapesDraftText(t *testing.T) {
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{{ID: "f-001", Rev: 1, Title: "ti\x1b[2Jtle", Body: "b", General: true, Included: true}}
	d.Notes = []draft.Note{
		{ID: "n-001", FindingID: "f-001", Body: "closed\u202e", Status: draft.NoteResolved},
		{ID: "n-002", FindingID: "f-001", Body: "open\x1b]52;c;x\a", Status: draft.NoteOpen},
	}
	d.Replies = []draft.Reply{{ID: "r-001", NoteID: "n-002", Body: "re\u2066ply", By: "agent"}}
	var out strings.Builder
	if err := printFeedback(&out, "o/r#1@1", d); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.ContainsAny(got, "\x1b\a\u202e\u2066") {
		t.Fatalf("feedback output carries a raw control:\n%q", got)
	}
	for _, want := range []string{`ti\u001B[2Jtle`, `open\u001B]52;c;x\u0007`, `re\u2066ply`, `closed\u202E`} {
		if !strings.Contains(got, want) {
			t.Errorf("feedback output lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "n-002") > strings.Index(got, "n-001") {
		t.Errorf("open notes are not listed first:\n%s", got)
	}
}
