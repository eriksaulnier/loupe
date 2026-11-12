package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/publish"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
)

// assessRun stores round 1 of o/r#1, published by reviewer with findings f-001 and f-002 under publication id pub,
// and an empty round 2 to assess them from. An empty pub records round 1 as an older loupe did.
func assessRun(t *testing.T, pub string) (home string) {
	t.Helper()
	home = t.TempDir()
	at := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	draftJSON, err := json.Marshal(draft.NewEmpty())
	if err != nil {
		t.Fatal(err)
	}
	for round := 1; round <= 2; round++ {
		target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: round, Viewer: "reviewer",
			URL: "https://github.com/o/r/pull/1", CapturedAt: at}
		if err := run.CreateRun(run.RunDir(home, "o", "r", 1, round), target, nil, draftJSON, nil); err != nil {
			t.Fatal(err)
		}
	}
	saveAssessReceipt(t, home, pub)
	return home
}

// saveAssessReceipt records round 1 as published under publication id pub.
func saveAssessReceipt(t *testing.T, home, pub string) {
	t.Helper()
	at := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	receipt := &publish.Receipt{Schema: 1, ReviewID: 7, ReviewURL: testReviewURL, Action: "comment", PostedAt: at, Author: "reviewer",
		Envelope: publish.Envelope{PublicationID: pub, Viewer: "reviewer", Body: "Review.\n\n<!-- loupe-meta v=1 -->",
			Target: publish.EnvelopeTarget{Owner: "o", Repo: "r", Number: 1, Round: 1},
			Findings: []publish.EnvelopeFinding{
				{ID: "f-001", Title: "One", Body: "Body.", Label: "issue"},
				{ID: "f-002", Title: "Two", Body: "Body.", Label: "issue"},
			}}}
	if err := publish.SaveReceipt(run.RunDir(home, "o", "r", 1, 1), receipt); err != nil {
		t.Fatal(err)
	}
}

func runAssessIn(t *testing.T, home, stdin string, args ...string) (int, *streams) {
	t.Helper()
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	deps.Stdin = strings.NewReader(stdin)
	return Execute(deps, append([]string{"assess", "--run", "o/r#1@2"}, args...)), s
}

func TestAssessFlagRefusals(t *testing.T) {
	home := assessRun(t, "pub-1")
	checkRefusals(t, home, []refusalCase{
		{"--from with --status", "", []string{"assess", "--run", "o/r#1@2", "--from", "-", "--status", "open"}, refusal.Usage, "--status"},
		{"refs and --from", "", []string{"assess", "--run", "o/r#1@2", "e-1", "--from", "-"}, refusal.Usage, "refs were given both as arguments and with --from"},
		{"no refs", "", []string{"assess", "--run", "o/r#1@2", "--status", "open"}, refusal.Usage, "no refs given"},
		{"bad status", "", []string{"assess", "--run", "o/r#1@2", "e-1", "--status", "done"}, refusal.Usage, `--status must be open or addressed, not "done"`},
		{"malformed input", "{", []string{"assess", "--run", "o/r#1@2", "--from", "-"}, refusal.Input, "input ends at byte 1"},
		{"no assessments", "{}", []string{"assess", "--run", "o/r#1@2", "--from", "-"}, refusal.Input, `input has no "assessments"`},
		{"stale version", "", []string{"assess", "--run", "o/r#1@2", "e-1", "--status", "open", "--expect-version", "5"}, refusal.Version, "draft is at version 0"},
	})
}

func TestAssessSaysWhatItAssessed(t *testing.T) {
	home := assessRun(t, "pub-1")
	code, s := runAssessIn(t, home, "", "e-1", "--status", "addressed")
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
	if want := "+ Assessed e-1 on o/r#1@2: 0 open, 1 addressed, 1 of 2 unassessed  draft version 1\n"; s.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", s.stdout.String(), want)
	}
	// Round 1 republished under another publication id is another previous round, so its assessment is dropped.
	saveAssessReceipt(t, home, "pub-2")
	code, s = runAssessIn(t, home, `{"assessments": []}`, "--from", "-")
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
	if want := "+ Assessed nothing on o/r#1@2: 2 of 2 unassessed; dropped 1 assessment read from an earlier previous round  draft version 2\n"; s.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", s.stdout.String(), want)
	}
}

func TestAssessRefusesAPreviousRoundWithoutAPublicationID(t *testing.T) {
	home := assessRun(t, "")
	code, s := runAssessIn(t, home, "", "e-1", "--status", "open", "--json")
	errObj := wantCode(t, code, s, refusal.NotFound)
	if !strings.Contains(errObj["message"].(string), "did not record its publication id") ||
		!strings.Contains(errObj["fix"].(string), "loupe capture https://github.com/o/r/pull/1") {
		t.Fatalf("error %v", errObj)
	}
}
