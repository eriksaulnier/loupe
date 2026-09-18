package publish

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// buildInput is the Build call these tests share: round 1, the reviewer, a fresh publication id and no message.
func buildInput(target run.Target, d *draft.Draft, action, inline string, unattended bool) BuildInput {
	return BuildInput{Target: target, Round: 1, Draft: d, Viewer: "reviewer", Action: action, Inline: inline,
		Unattended: unattended, PublicationID: newPublicationID()}
}

// TestBuildKeepsThePublicationIDItIsGiven pins the marker a human approves to the marker that is sent: the
// confirmation composes the review once to show it and again for the message, and Reconcile finds an interrupted
// publish only by that exact string.
func TestBuildKeepsThePublicationIDItIsGiven(t *testing.T) {
	in := buildInput(fixtureTarget(), readyDraft(), "comment", "none", false)
	first, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Message = "Looks good to me."
	second, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if first.PublicationID != in.PublicationID || second.PublicationID != in.PublicationID {
		t.Fatalf("publication ids %q and %q, want %q", first.PublicationID, second.PublicationID, in.PublicationID)
	}
	marker := "publication=" + in.PublicationID + " -->"
	if !strings.Contains(first.Body, marker) || !strings.Contains(second.Body, marker) {
		t.Fatalf("bodies lack the marker %q:\n%s\n%s", marker, first.Body, second.Body)
	}
}

// TestBuildOpensOnTheMessageWhenAttended is the feature: the human's words lead an attended review and the agent's
// summary leads an unattended one.
func TestBuildOpensOnTheMessageWhenAttended(t *testing.T) {
	d := readyDraft()
	d.Summary = "The agent wrote this."
	in := buildInput(fixtureTarget(), d, "comment", "none", false)
	in.Message = "I read every one of these."
	env, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Body, "I read every one of these.") {
		t.Fatalf("attended body lacks the message:\n%s", env.Body)
	}
	if strings.Contains(env.Body, "The agent wrote this.") {
		t.Fatalf("attended body carries the draft summary:\n%s", env.Body)
	}

	unattended := buildInput(fixtureTarget(), d, "comment", "none", true)
	unattended.Message = "nobody typed this"
	env, err = Build(unattended)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Body, "The agent wrote this.") {
		t.Fatalf("unattended body lacks the draft summary:\n%s", env.Body)
	}
	if strings.Contains(env.Body, "nobody typed this") {
		t.Fatalf("unattended body carries a message:\n%s", env.Body)
	}
}

// TestBuildEmptyMessageRendersWhatAnEmptySummaryRenders is SC-003. The golden was rendered by the code before the
// message existed, from a draft whose summary was empty, with the publication id fixed: a review published with no
// message is that review, byte for byte, so the common path cost nothing and no new branch reached the renderer.
func TestBuildEmptyMessageRendersWhatAnEmptySummaryRenders(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000000"
	blank := readyDraft()
	blank.Summary = ""
	in := buildInput(fixtureTarget(), blank, "comment", "all", false)
	in.PublicationID = id
	empty, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "publish", "no-opening-body.md"))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Body != string(want) {
		t.Fatalf("a body with no opening changed:\n%s", empty.Body)
	}
	if first, _, _ := strings.Cut(empty.Body, "\n"); !strings.Contains(first, "blocking") {
		t.Fatalf("the body does not begin at the chips row: %q", first)
	}

	// The same holds when the draft does have a summary, because an attended review never reads it.
	in.Draft = readyDraft()
	withSummary, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	// The two drafts differ, so their digests do, and the digest is in the marker the footer ends on.
	const marker = "<!-- loupe digest="
	head, _, _ := strings.Cut(withSummary.Body, marker)
	if got, _, _ := strings.Cut(empty.Body, marker); got != head {
		t.Fatalf("the stored summary reached an attended body:\n%q\n%q", head, got)
	}
}

func TestBuildComposesEnvelope(t *testing.T) {
	d := readyDraft()
	in := buildInput(fixtureTarget(), d, "request-changes", "all", false)
	in.Message = "Mine to own."
	env, err := Build(in)
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
	// An attended review opens on the human's message, never on the draft's summary.
	want := render.Input{Owner: "acme", Repo: "widgets", Number: 42, Round: 1, HeadSHA: headSHA, Inline: "all",
		Summary: "Mine to own.", Digest: draft.Digest(d), PublicationID: env.PublicationID, Findings: fs}
	if env.Body != render.Body(want) {
		t.Fatalf("body differs from render.Body:\n%s", env.Body)
	}
	marker := "\n<!-- loupe digest=" + draft.Digest(d) + " publication=" + env.PublicationID + " -->\n"
	if !strings.Contains(env.Body, marker) {
		t.Fatalf("body lacks marker line %q", marker)
	}
	var wantComments []Comment
	for _, c := range render.Comments(want) {
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
		env, err := Build(buildInput(fixtureTarget(), readyDraft(), action, "none", false))
		if err != nil {
			t.Fatal(err)
		}
		if env.Event != event || len(env.Comments) != 0 || env.Comments == nil {
			t.Errorf("action %s: event %q comments %#v", action, env.Event, env.Comments)
		}
	}
	env, err := Build(buildInput(fixtureTarget(), readyDraft(), "comment", "blocking", false))
	if err != nil {
		t.Fatal(err)
	}
	if len(env.Comments) != 1 || env.Comments[0].Line != 3 {
		t.Fatalf("blocking comments %+v", env.Comments)
	}
	if _, err := Build(buildInput(fixtureTarget(), readyDraft(), "merge", "none", false)); err == nil {
		t.Fatal("unknown action accepted")
	}
	if _, err := Build(buildInput(fixtureTarget(), readyDraft(), "comment", "some", false)); err == nil {
		t.Fatal("unknown inline mode accepted")
	}
}

func TestBuildRechecksMarkdown(t *testing.T) {
	d := readyDraft()
	d.Summary = "<script>alert(1)</script>"
	// An attended publication does not post the stored summary, so a malformed one is not its problem.
	if _, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false)); err != nil {
		t.Fatalf("attended build refused over a summary it will not publish: %v", err)
	}
	_, err := Build(buildInput(fixtureTarget(), d, "comment", "none", true))
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Markdown || r.Fix != "loupe summary --from -" {
		t.Fatalf("unattended summary: %#v", err)
	}

	d = readyDraft()
	bad := buildInput(fixtureTarget(), d, "comment", "none", false)
	bad.Message = "<script>alert(1)</script>"
	_, err = Build(bad)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Markdown || r.Fix != messageFix {
		t.Fatalf("message: %#v", err)
	}

	d = readyDraft()
	d.Findings[1].Body = "<pre>raw</pre>"
	_, err = Build(buildInput(fixtureTarget(), d, "comment", "none", false))
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Markdown || r.Fix != "loupe edit f-002 --from -" {
		t.Fatalf("accepted body: %#v", err)
	}

	d = readyDraft()
	d.Findings[2].Body = "<pre>raw</pre>"
	d.Findings[3].Body = "<pre>raw</pre>"
	if _, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false)); err != nil {
		t.Fatalf("excluded or withdrawn body was checked: %v", err)
	}
}

func TestBuildCarriesSource(t *testing.T) {
	target := fixtureTarget()
	target.Source = "gadfly-review-pr@2.2.0"
	env, err := Build(buildInput(target, readyDraft(), "comment", "none", false))
	if err != nil || !strings.Contains(env.Body, " · via `gadfly-review-pr 2.2.0`") || !strings.Contains(env.Body, " src=gadfly-review-pr@2.2.0 ") {
		t.Fatalf("source not rendered: %v\n%s", err, env.Body)
	}
}

func TestBuildCarriesModel(t *testing.T) {
	target := fixtureTarget()
	target.Model = "anthropic/claude-sonnet-5"
	env, err := Build(buildInput(target, readyDraft(), "comment", "none", false))
	if err != nil || strings.Contains(env.Body, "claude-sonnet-5`") || !strings.Contains(env.Body, " model=anthropic/claude-sonnet-5 ") {
		t.Fatalf("model not rendered: %v\n%s", err, env.Body)
	}
}

func TestBuildCarriesFindingFields(t *testing.T) {
	d := readyDraft()
	f := &d.Findings[0]
	f.Severity, f.Verified, f.Impact, f.References = "major", "plausible", "Breaks.", []string{"https://github.com/o/r/issues/1"}
	env, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false))
	if err != nil {
		t.Fatal(err)
	}
	// The severity is on the summary line, not in the meta block, so the block carries verified alone.
	for _, want := range []string{"<b>major · ", "> **Verified:** plausible", "**Impact**\n\nBreaks.", "**References**\n\n- <https://github.com/o/r/issues/1>"} {
		if !strings.Contains(env.Body, want) {
			t.Fatalf("missing %q in\n%s", want, env.Body)
		}
	}
	f.Impact = "one<br>two"
	if _, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false)); err == nil {
		t.Fatal("impact failing the allowlist was composed")
	}
	f.Impact, f.References = "", []string{"https://github.com/o/r/</details>"}
	_, err = Build(buildInput(fixtureTarget(), d, "comment", "none", false))
	if r, ok := refusal.As(err); !ok || r.Fix != "loupe edit "+f.ID+" --from -" {
		t.Fatalf("a reference that could close the disclosure: got %v, want a refusal whose fix names the finding", err)
	}
}

func TestBuildRefusesBodyOverLimit(t *testing.T) {
	d := readyDraft()
	long := strings.Repeat(strings.Repeat("a", 99)+"\n", 600)
	d.Summary = long
	for _, i := range []int{0, 1, 4} {
		d.Findings[i].Body, d.Findings[i].SuggestedFix = long, long
	}
	_, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false))
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Markdown || r.Details["rule"] != "limit" || r.Fix != "exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -" {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildCountsBodyLimitInCharacters(t *testing.T) {
	d := readyDraft()
	wide := strings.Repeat("é", 30000)
	d.Findings[0].Body, d.Findings[1].Body = wide, wide
	if _, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false)); err != nil {
		t.Fatalf("a body of about 60,000 two-byte characters was refused: %v", err)
	}
	d.Findings[4].Body = wide
	_, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false))
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Markdown || r.Details["rule"] != "limit" || !strings.Contains(r.Message, "65536 characters") {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildRefusesInlineCommentOverLimit(t *testing.T) {
	d := readyDraft()
	d.Findings[0].Body, d.Findings[0].SuggestedFix = strings.Repeat("a", 60000), strings.Repeat("b", 10000)
	_, err := Build(buildInput(fixtureTarget(), d, "comment", "blocking", false))
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Markdown || r.Details["rule"] != "limit" || !strings.Contains(r.Message, "f-001") ||
		r.Fix != "exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -" {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildRefusesWhenPublishableSetIsNotAccepted(t *testing.T) {
	d := readyDraft()
	delete(d.Decisions, "f-002")
	_, err := Build(buildInput(fixtureTarget(), d, "comment", "none", false))
	if r, ok := refusal.As(err); !ok || r.Code != refusal.NotReady {
		t.Fatalf("err %#v", err)
	}
}

func TestBuildUnattendedComposesFromPublishableSet(t *testing.T) {
	d := readyDraft()
	delete(d.Decisions, "f-002")
	env, err := Build(buildInput(fixtureTarget(), d, "comment", "none", true))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, f := range env.Findings {
		ids = append(ids, f.ID)
	}
	if !reflect.DeepEqual(ids, []string{"f-001", "f-002", "f-005"}) {
		t.Fatalf("findings %+v, want the pending f-002 included", ids)
	}
	assertOnlyAccepted(t, mustJSON(t, env))
}

// TestBuildUnattendedBodyIsUnchanged is SC-004. The golden was produced by the code before the human's message
// existed, with the publication id fixed; an unattended round posts what it has always posted.
func TestBuildUnattendedBodyIsUnchanged(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000000"
	in := buildInput(fixtureTarget(), readyDraft(), "comment", "all", true)
	in.Viewer, in.PublicationID = "", id
	env, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "publish", "unattended-body.md"))
	if err != nil {
		t.Fatal(err)
	}
	if env.Body != string(want) {
		t.Fatalf("the unattended body changed:\n%s", env.Body)
	}
}

func TestBuildUnattendedStillRechecksMarkdown(t *testing.T) {
	d := readyDraft()
	delete(d.Decisions, "f-002")
	d.Findings[1].Body = "<pre>raw</pre>"
	_, err := Build(buildInput(fixtureTarget(), d, "comment", "none", true))
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Markdown || r.Fix != "loupe edit f-002 --from -" {
		t.Fatalf("pending body: %#v", err)
	}
}

// The human decides findings in draft.Ordered and the pull request reader receives render.Body. This is the seam
// where the two meet, and the whole claim of the shared order is that the sequence is the same on both sides.
func TestComposedBodyFollowsTheOrderTheHumanDecidedIn(t *testing.T) {
	d := draft.NewEmpty()
	d.Version = 7
	d.Summary = "Looks mostly fine."
	add := func(id, label, sev string, blocking bool) draft.Finding {
		f := finding(id, label, blocking, nil)
		f.Title, f.Body, f.Severity = "Title "+id, "Body "+id+".", sev
		return f
	}
	// Section placement and severity disagree on purpose: a nonblocking critical outranks a blocking minor by
	// severity alone, and the body puts the blocking one first regardless.
	d.Findings = []draft.Finding{
		add("f-001", "issue", "critical", false),
		add("f-002", "issue", "minor", true),
		add("f-003", "question", "critical", true),
		add("f-004", "suggestion", "", false),
		add("f-005", "perf-nit", "trivial", false),
		add("f-006", "issue", "", false),
		add("f-007", "suggestion", "major", true),
	}
	for _, f := range d.Findings {
		accept(d, f.ID)
	}
	env, err := Build(fixtureTarget(), 1, d, "reviewer", "comment", "none", false)
	if err != nil {
		t.Fatal(err)
	}
	at := -1
	for _, f := range draft.Ordered(d) {
		i := strings.Index(env.Body, "Title "+f.ID+"<")
		if i < 0 {
			t.Fatalf("%s is not in the composed body:\n%s", f.ID, env.Body)
		}
		if i < at {
			t.Fatalf("%s appears at %d, before a finding the human meets earlier:\n%s", f.ID, i, env.Body)
		}
		at = i
	}
}
