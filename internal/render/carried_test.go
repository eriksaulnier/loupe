package render

import (
	"strings"
	"testing"
)

const filingCommit = "37f7b19dbab6df35a826451bb0350d12c78f609e"

func carried(ref, status, label string, blocking bool, loc *RecordLocation) RecordAssessment {
	id := "f-00" + ref[len(ref)-1:]
	return RecordAssessment{Ref: ref, Status: status, Finding: RecordEarlier{
		RecordFinding: RecordFinding{ID: id, Title: "Title " + ref, Body: "Body " + ref + ".", Location: loc, Label: label, Blocking: blocking},
		FiledIn:       RecordFiledIn{Round: 1, ReviewURL: "https://github.com/o/r/pull/7#pullrequestreview-1", Commit: filingCommit},
	}}
}

// carriedInput is a round with prose and no finding of its own, so its chips and rows come from the assessments alone.
func carriedInput(assessments ...RecordAssessment) Input {
	in := exampleInput()
	in.Summary, in.Findings, in.Assessments = "Summary.", nil, assessments
	return in
}

func TestCarriedFindingCountsInTheChipsRow(t *testing.T) {
	body := Body(carriedInput(carried("e-1", "open", "suggestion", false, nil)))
	if !strings.HasPrefix(body, "`🟣 1 suggestion`\n\n") {
		t.Fatalf("an open earlier suggestion must count in the chips row\n%s", body)
	}
	in := carriedInput(carried("e-1", "open", "issue", true, nil), carried("e-2", "open", "suggestion", false, nil))
	in.Findings = []Finding{general("f-009", "suggestion", false)}
	if body := Body(in); !strings.HasPrefix(body, "`⛔ 1 blocking` `🟣 2 suggestions`\n\n") {
		t.Fatalf("carried findings must count in the chip their row dot keys\n%s", body)
	}
}

func TestAddressedFindingLeavesTheRoundClean(t *testing.T) {
	body := Body(carriedInput(carried("e-1", "addressed", "issue", true, nil)))
	if !strings.HasPrefix(body, "`"+cleanChip+"`\n\n") || strings.Contains(body, "Still open") {
		t.Fatalf("an addressed finding must leave the round clean and unlisted\n%s", body)
	}
	_, record, _ := tail(t, body)
	if plain := Body(carriedInput()); body != strings.Replace(plain, recordOf(t, plain), record, 1) {
		t.Fatal("an addressed finding must change only the record line")
	}
}

func TestCarriedSection(t *testing.T) {
	in := carriedInput(
		carried("e-1", "open", "question", false, &RecordLocation{Path: "a b/c.go", Side: "RIGHT", Line: 14, StartLine: 10}),
		carried("e-2", "open", "suggestion", false, &RecordLocation{Path: "old.go", Side: "LEFT", Line: 24}),
		carried("e-3", "addressed", "issue", true, nil),
		carried("e-4", "open", "", true, nil),
		carried("e-5", "open", "issue", false, &RecordLocation{Path: "x.go", Side: "RIGHT", Line: 3}),
	)
	in.Findings = []Finding{general("f-009", "suggestion", false)}
	want := "### Worth a look\n\n<details>\n<summary>🟣 <b>suggestion</b>: Title f-009</summary>\n\nBody f-009.\n\n</details>\n\n---\n\n" +
		"### Still open from earlier rounds\n\n" +
		"- ⛔ Title e\\-4 · filed at `37f7b19`\n" +
		"- 🟡 <b>issue</b>: Title e\\-5 · [`x.go:3`](https://github.com/o/r/blob/" + filingCommit + "/x.go#L3) · filed at `37f7b19`\n" +
		"- 🟣 <b>suggestion</b>: Title e\\-2 · `old.go:24 (LEFT)` · filed at `37f7b19`\n" +
		"- 🔵 <b>question</b>: Title e\\-1 · [`a b/c.go:10–14`](https://github.com/o/r/blob/" + filingCommit + "/a%20b/c.go#L10-L14) · filed at `37f7b19`\n\n---\n\nreviewed "
	if body := Body(in); !strings.Contains(body, want) {
		t.Fatalf("carried section wrong\n--- got ---\n%s\n--- want within ---\n%s", body, want)
	}
}

// The record is text anyone who can edit the review controls, so a carried field gets no more trust than a finding's.
func TestCarriedSectionEscapesTheRecord(t *testing.T) {
	a := carried("e-1", "open", "*perf*", false, &RecordLocation{Path: "p.go", Side: "RIGHT", Line: 1})
	a.Finding.Title = "Closes </summary> & *em* `code`"
	a.Finding.FiledIn.Commit = "abc) [x](evil"
	body := Body(carriedInput(a))
	want := "- ⚪ <b>\\*perf\\*</b>: Closes &lt;\\/summary&gt; &amp; \\*em\\* <code>code</code> · `p.go:1`\n"
	if !strings.Contains(body, want) {
		t.Fatalf("carried row not escaped, or linked to a commit that is not hex\n--- got ---\n%s\n--- want within ---\n%s", body, want)
	}
}

func TestCarriedMeta(t *testing.T) {
	in := carriedInput(carried("e-1", "open", "issue", true, nil), carried("e-2", "open", "question", false, nil),
		carried("e-3", "addressed", "issue", true, nil))
	in.Sticky = &StickyInput{Rounds: 1}
	_, _, meta := tail(t, Body(in))
	if !strings.HasSuffix(meta, " blocking=0 issues=0 suggestions=0 questions=0 other=0 excluded=0 withdrawn=0 reinstated=0 regraded=0 carried=2 carriedblocking=1 sticky=1 -->") {
		t.Fatalf("loupe-meta must keep its census and name the carried findings before sticky=\n%s", meta)
	}
	if _, _, meta := tail(t, Body(carriedInput(carried("e-1", "addressed", "issue", true, nil)))); strings.Contains(meta, "carried") {
		t.Fatalf("loupe-meta must name no carried finding when none is open\n%s", meta)
	}
}

func TestStickyRoundKeepsItsCarriedFindingsWhenCollapsed(t *testing.T) {
	in := carriedInput(carried("e-1", "open", "suggestion", false, nil))
	in.Inline, in.Sticky = "none", &StickyInput{Rounds: 1}
	// Only the anchored reader: loupe wrote no carried section before anchors, so no legacy body holds one.
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	block := earlier[0].Block
	if !strings.HasPrefix(block, "<details>\n<summary>Round 1 · reviewed <code>d23632e</code> · <code>🟣 1 suggestion</code></summary>\n\n") ||
		!strings.Contains(block, "\n>\n> ### Still open from earlier rounds\n>\n> - 🟣 <b>suggestion</b>: Title e\\-1 · filed at `37f7b19`\n>\n> reviewed ") {
		t.Fatalf("collapsed round\n%s", block)
	}
}

// carriedGoldenInput is a second sticky round that files nothing and marks round 1's blocking issue open.
func carriedGoldenInput(t *testing.T) Input {
	t.Helper()
	earlier, rounds, err := ReadSticky(firstRound())
	if err != nil {
		t.Fatal(err)
	}
	in := stickyInput(2, "bbbbbbb222")
	in.Summary = "The change does not touch the blocking issue."
	in.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	a := carried("e-1", "open", "issue", true, nil)
	a.Finding.ID, a.Finding.Title, a.Finding.FiledIn.Commit = "f-001", "Title f-001", "aaaaaaa111"
	in.Assessments = []RecordAssessment{a}
	return in
}
