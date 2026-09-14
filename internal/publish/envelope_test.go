package publish

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
)

func TestBuildComposesEnvelope(t *testing.T) {
	d := readyDraft()
	env, err := Build(fixtureTarget(), 1, d, "reviewer", "request-changes", "all")
	if err != nil {
		t.Fatal(err)
	}
	if env.Target != (EnvelopeTarget{Owner: "acme", Repo: "widgets", Number: 42, HeadSHA: headSHA, Round: 1}) ||
		env.Viewer != "reviewer" || env.Action != "request-changes" || env.Event != "REQUEST_CHANGES" || env.CommitID != headSHA ||
		env.DraftVersion != 7 || env.Digest != draft.Digest(d) || env.Inline != "all" || !uuidV4.MatchString(env.PublicationID) {
		t.Fatalf("envelope header %+v", env)
	}

	var fs []render.Finding
	for _, id := range []int{0, 1, 4} {
		f := d.Findings[id]
		rf := render.Finding{ID: f.ID, Title: f.Title, Body: f.Body, General: f.General, Label: f.Label, Blocking: f.Blocking}
		if f.Location != nil {
			rf.Location = &render.Location{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
		}
		fs = append(fs, rf)
	}
	in := render.Input{Owner: "acme", Repo: "widgets", Number: 42, Round: 1, HeadSHA: headSHA, Inline: "all",
		Summary: d.Summary, Digest: draft.Digest(d), PublicationID: env.PublicationID, Findings: fs}
	if env.Body != render.Body(in) {
		t.Fatalf("body differs from render.Body:\n%s", env.Body)
	}
	marker := "\n<!-- loupe digest=" + draft.Digest(d) + " publication=" + env.PublicationID + " -->\n"
	if !strings.Contains(env.Body, marker) {
		t.Fatalf("body lacks marker line %q", marker)
	}
	var wantComments []Comment
	for _, c := range render.Comments(in) {
		wantComments = append(wantComments, Comment{Path: c.Path, Line: c.Line, Side: c.Side, StartLine: c.StartLine, StartSide: c.StartSide, Body: c.Body})
	}
	if len(wantComments) != 2 || !reflect.DeepEqual(env.Comments, wantComments) {
		t.Fatalf("comments %+v", env.Comments)
	}
	var ids []string
	for _, f := range env.Findings {
		ids = append(ids, f.ID)
	}
	if !reflect.DeepEqual(ids, []string{"f-001", "f-002", "f-005"}) || env.Findings[1].Location.StartLine != 10 || env.Findings[2].Location != nil {
		t.Fatalf("findings %+v", env.Findings)
	}
	assertOnlyAccepted(t, mustJSON(t, env))
}

// assertOnlyAccepted checks that no text of the excluded f-003 or the withdrawn f-004 reached a payload.
func assertOnlyAccepted(t *testing.T, payload string) {
	t.Helper()
	for _, banned := range []string{"EXCLUDED", "WITHDRAWN", "f-003", "f-004"} {
		if strings.Contains(payload, banned) {
			t.Errorf("payload contains %q:\n%s", banned, payload)
		}
	}
}

func TestBuildEventAndInlineModes(t *testing.T) {
	for action, event := range map[string]string{"comment": "COMMENT", "approve": "APPROVE", "request-changes": "REQUEST_CHANGES"} {
		env, err := Build(fixtureTarget(), 1, readyDraft(), "reviewer", action, "none")
		if err != nil {
			t.Fatal(err)
		}
		if env.Event != event || len(env.Comments) != 0 || env.Comments == nil {
			t.Errorf("action %s: event %q comments %#v", action, env.Event, env.Comments)
		}
	}
	env, err := Build(fixtureTarget(), 1, readyDraft(), "reviewer", "comment", "blocking")
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Comments) != 1 || env.Comments[0].Line != 3 {
		t.Fatalf("blocking comments %+v", env.Comments)
	}
	if _, err := Build(fixtureTarget(), 1, readyDraft(), "reviewer", "merge", "none"); err == nil {
		t.Fatal("unknown action accepted")
	}
	if _, err := Build(fixtureTarget(), 1, readyDraft(), "reviewer", "comment", "some"); err == nil {
		t.Fatal("unknown inline mode accepted")
	}
}

func TestBuildRechecksMarkdown(t *testing.T) {
	d := readyDraft()
	d.Summary = "<script>alert(1)</script>"
	_, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none")
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Markdown || r.Fix != "loupe summary --from -" {
		t.Fatalf("summary: %#v", err)
	}

	d = readyDraft()
	d.Findings[1].Body = "<pre>raw</pre>"
	_, err = Build(fixtureTarget(), 1, d, "reviewer", "comment", "none")
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Markdown || r.Fix != "loupe edit f-002 --from -" {
		t.Fatalf("accepted body: %#v", err)
	}

	d = readyDraft()
	d.Findings[2].Body = "<pre>raw</pre>"
	d.Findings[3].Body = "<pre>raw</pre>"
	if _, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none"); err != nil {
		t.Fatalf("excluded or withdrawn body was checked: %v", err)
	}
}

func TestBuildCarriesSource(t *testing.T) {
	target := fixtureTarget()
	target.Source = "gadfly-review-pr@2.2.0"
	env, err := Build(target, 1, readyDraft(), "reviewer", "comment", "none")
	if err != nil || !strings.Contains(env.Body, " · via `gadfly-review-pr 2.2.0`") || !strings.Contains(env.Body, " src=gadfly-review-pr@2.2.0 ") {
		t.Fatalf("source not rendered: %v\n%s", err, env.Body)
	}
}

func TestBuildRefusesBodyOverLimit(t *testing.T) {
	d := readyDraft()
	long := strings.Repeat(strings.Repeat("a", 99)+"\n", 600)
	d.Summary = long
	for _, i := range []int{0, 1, 4} {
		d.Findings[i].Body, d.Findings[i].SuggestedFix = long, long
	}
	_, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Markdown || r.Details["rule"] != "limit" || r.Fix != "exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -" {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildCountsBodyLimitInCharacters(t *testing.T) {
	d := readyDraft()
	wide := strings.Repeat("é", 30000)
	d.Findings[0].Body, d.Findings[1].Body = wide, wide
	if _, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none"); err != nil {
		t.Fatalf("a body of about 60,000 two-byte characters was refused: %v", err)
	}
	d.Findings[4].Body = wide
	_, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Markdown || r.Details["rule"] != "limit" || !strings.Contains(r.Message, "65536 characters") {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildRefusesInlineCommentOverLimit(t *testing.T) {
	d := readyDraft()
	d.Findings[0].Body, d.Findings[0].SuggestedFix = strings.Repeat("a", 60000), strings.Repeat("b", 10000)
	_, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "blocking")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Markdown || r.Details["rule"] != "limit" || !strings.Contains(r.Message, "f-001") ||
		r.Fix != "exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -" {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildRefusesWhenPublishableSetIsNotAccepted(t *testing.T) {
	d := readyDraft()
	delete(d.Decisions, "f-002")
	_, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none")
	if r, ok := refusal.As(err); !ok || r.Code != refusal.NotReady {
		t.Fatalf("err %#v", err)
	}
}
