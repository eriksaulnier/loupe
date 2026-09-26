package render

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func stickyInput(round int, sha string, findings ...Finding) Input {
	in := exampleInput()
	in.Round, in.HeadSHA, in.Inline, in.Summary, in.Findings = round, sha, "none", "", findings
	in.Digest = strings.Repeat(string(rune('0'+round)), 64)
	in.PublicationID = "00000000-0000-4000-8000-00000000000" + string(rune('0'+round))
	return in
}

// firstRound is a sticky body holding one round, as the first sticky publication posts it.
func firstRound() string {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
	in.Sticky = &StickyInput{Rounds: 1}
	return Body(in)
}

// stickyGoldenInput is a second round edited over a first, each with a finding and the second with a source.
func stickyGoldenInput(t *testing.T) Input {
	t.Helper()
	earlier, rounds, err := ReadSticky(firstRound())
	if err != nil {
		t.Fatal(err)
	}
	in := stickyInput(2, "bbbbbbb222", general("f-001", "question", false))
	in.Summary, in.Source = "The blocking issue is fixed; one question left.", "gadfly-review-pr@2.2.0"
	in.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	return in
}

// stickyThreeGoldenInput is a third round edited over the second golden's body, with both sections and a location.
func stickyThreeGoldenInput(t *testing.T) Input {
	t.Helper()
	earlier, rounds, err := ReadSticky(Body(stickyGoldenInput(t)))
	if err != nil {
		t.Fatal(err)
	}
	loc := Finding{ID: "f-002", Title: "Title f-002", Body: "Body f-002.", Label: "suggestion",
		Location: &Location{Path: "internal/a.go", Side: "RIGHT", Line: 12}}
	in := stickyInput(3, "ccccccc333", general("f-001", "issue", true), loc)
	in.Summary, in.Source = "A new blocking issue, and one suggestion.", "gadfly-review-pr@2.3.0"
	in.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	return in
}

func TestStickyOffLeavesBodyUnchanged(t *testing.T) {
	in := exampleInput()
	plain := Body(in)
	in.Sticky = &StickyInput{Rounds: 1}
	sticky := Body(in)
	// The record's checksum covers loupe-meta, so it differs too, and is compared without it.
	plain, sticky = strings.Replace(plain, recordOf(t, plain)+"\n", "", 1), strings.Replace(sticky, recordOf(t, sticky)+"\n", "", 1)
	if want := strings.Replace(plain, " regraded=0 -->", " regraded=0 sticky=1 -->", 1); sticky != want {
		t.Fatalf("a one-round sticky body must differ only by sticky=1\n--- got ---\n%s\n--- want ---\n%s", sticky, want)
	}
	if strings.Contains(plain, "sticky=") {
		t.Fatal("a non-sticky body carries sticky=")
	}
}

func TestStickyBodyKeepsEachFooterUnderItsRound(t *testing.T) {
	earlier, rounds, err := ReadSticky(firstRound())
	if err != nil || rounds != 1 || len(earlier) != 1 {
		t.Fatalf("ReadSticky: %d blocks, rounds %d, err %v", len(earlier), rounds, err)
	}
	in := stickyInput(2, "bbbbbbb222", general("f-001", "question", false))
	in.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	body := Body(in)
	at := indexes(t, body, "### Worth a look", "</details>\n\n---\n\nreviewed [`bbbbbbb`](https://github.com/o/r/commit/bbbbbbb222) · [changes since round 1](https://github.com/o/r/compare/aaaaaaa111...bbbbbbb222)\n\n---\n\n<!-- loupe-earlier -->\n\n### Earlier rounds\n\n<!-- loupe-round -->\n\n<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>⛔ 1 blocking</code></summary>",
		"> </details>\n>\n> reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n<!-- loupe digest=1", "</details>\n\n<!-- loupe digest=2", "sticky=2 -->")
	if at[0] > at[1] || at[1] > at[2] || at[2] > at[3] || at[3] > at[4] {
		t.Fatalf("sections out of order %v:\n%s", at, body)
	}
	if strings.Contains(body, "dropped") {
		t.Fatalf("no round was dropped:\n%s", body)
	}
}

func TestStickyBodyCountsDroppedRounds(t *testing.T) {
	for _, c := range []struct {
		rounds, kept int
		note         string
	}{
		{3, 1, "The oldest round was dropped to fit GitHub's length limit."},
		{5, 1, "The 3 oldest rounds were dropped to fit GitHub's length limit."},
		{3, 0, "The 2 oldest rounds were dropped to fit GitHub's length limit."},
	} {
		earlier, _, err := ReadSticky(firstRound())
		if err != nil {
			t.Fatal(err)
		}
		in := stickyInput(c.rounds, "ccccccc333")
		in.Summary = "Prose."
		in.Sticky = &StickyInput{Rounds: c.rounds, Earlier: earlier[:c.kept]}
		body := Body(in)
		if !strings.Contains(body, "### Earlier rounds\n\n"+c.note+"\n\n") {
			t.Errorf("rounds %d kept %d: note missing:\n%s", c.rounds, c.kept, body)
		}
	}
}

func TestReadStickyDemotesTheCurrentRound(t *testing.T) {
	earlier, _, err := ReadSticky(firstRound())
	if err != nil {
		t.Fatal(err)
	}
	// The chips move into the summary, and the rest of the round with its footer becomes one quote with no divider. The
	// reconciliation marker stays outside the quote, so an interrupted publish of this round still reconciles.
	want := "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>⛔ 1 blocking</code></summary>\n\n" +
		"> ### Must fix\n>\n> <details>\n> <summary>⛔ <b>issue</b>: Title f-001</summary>\n>\n> Body f-001.\n>\n> </details>\n>\n" +
		"> reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n<!-- loupe digest=" + strings.Repeat("1", 64) + " publication=00000000-0000-4000-8000-000000000001 -->\n\n</details>"
	if earlier[0] != want {
		t.Fatalf("demoted round\n--- got ---\n%s\n--- want ---\n%s", earlier[0], want)
	}
}

// Rounds are numbered by their place in the sticky review, not by loupe's round count, which can start above 1.
func TestReadStickyNumbersRoundsInTheReview(t *testing.T) {
	in := stickyInput(5, "aaaaaaa111", general("f-001", "question", false))
	in.Summary = "Prose stays.\n\n---\n\nAfter a break."
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>🔵 1 question</code></summary>\n\n> Prose stays.\n>\n> ---\n>\n> After a break.\n>\n> ### Worth a look") {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
	next := stickyInput(6, "bbbbbbb222")
	next.Summary = "Nothing left."
	next.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	again, _, err := ReadSticky(Body(next))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(again[0], "<details>\n<summary>Round 2 · reviewed <code>bbbbbbb</code> · <code>🟢 no findings</code></summary>\n\n> Nothing left.\n>\n> reviewed [`bbbbbbb`](https://github.com/o/r/commit/bbbbbbb222) · [changes since round 1](https://github.com/o/r/compare/aaaaaaa111...bbbbbbb222)\n\n<!-- loupe digest=") ||
		!strings.HasPrefix(again[1], "<details>\n<summary>Round 1 · ") {
		t.Fatalf("blocks:\n%s", strings.Join(again, "\n=====\n"))
	}
}

// A body written before the collapsed-round format changed still reads back, and its old rounds are renumbered.
func TestReadStickyReadsTheEarlierFormat(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sticky-before-025-format.md"))
	if err != nil {
		t.Fatal(err)
	}
	old := strings.Replace(string(data), "<summary>Round 1 · ", "<summary>Round 3 · ", 1)
	earlier, rounds, err := ReadSticky(old)
	if err != nil || rounds != 2 || len(earlier) != 2 {
		t.Fatalf("blocks %d rounds %d err %v", len(earlier), rounds, err)
	}
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 2 · reviewed <code>bbbbbbb</code> · <code>🔵 1 question</code></summary>") ||
		!strings.HasPrefix(earlier[1], "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code></summary>") ||
		!strings.Contains(earlier[1], "reviewed `aaaaaaa`") {
		t.Fatalf("blocks:\n%s", strings.Join(earlier, "\n=====\n"))
	}
}

func TestReadStickyKeepsOlderBlocksNewestFirst(t *testing.T) {
	earlier, _, err := ReadSticky(firstRound())
	if err != nil {
		t.Fatal(err)
	}
	second := stickyInput(2, "bbbbbbb222", general("f-001", "question", false))
	second.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	// A dropped-rounds note in the body being read is regenerated, never carried.
	second.Sticky.Rounds = 4
	again, rounds, err := ReadSticky(Body(second))
	if err != nil || rounds != 4 {
		t.Fatalf("rounds %d err %v", rounds, err)
	}
	if len(again) != 2 || !strings.Contains(again[0], "Round 4 · reviewed <code>bbbbbbb</code>") || again[1] != strings.Replace(earlier[0], "Round 1 · ", "Round 3 · ", 1) {
		t.Fatalf("blocks %d:\n%s", len(again), strings.Join(again, "\n=====\n"))
	}
	for _, b := range again {
		if strings.Contains(b, "dropped") || strings.Contains(b, MetaPrefix) || strings.Contains(b, "loupe-round") {
			t.Fatalf("block carries a generated line it must not:\n%s", b)
		}
	}
}

// A finding that quotes the delimiters inside a fence, as a review of loupe itself might, moves no boundary.
func TestReadStickyIgnoresFencedDelimiters(t *testing.T) {
	quoted := "```\n<!-- loupe-earlier -->\n<!-- loupe-round -->\n" + MetaPrefix + "v=1 round=9 sticky=9 -->\n```"
	in := stickyInput(1, "aaaaaaa111", Finding{ID: "f-001", Title: "Quotes markers", Body: quoted, General: true})
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, rounds, err := ReadSticky(Body(in))
	// The demoted round is quoted, fence and all.
	if err != nil || rounds != 1 || len(earlier) != 1 || !strings.Contains(earlier[0], "> "+strings.ReplaceAll(quoted, "\n", "\n> ")) {
		t.Fatalf("rounds %d blocks %d err %v", rounds, len(earlier), err)
	}
	if got, sticky := StickyRounds(quoted); got != 0 || sticky {
		t.Fatalf("StickyRounds of a fenced marker = %d, want 0", got)
	}
}

// The footer can carry a source, a model and the unattended mark after the commit. Only the commit reaches the summary,
// and the demoted round keeps the whole footer, so the history shows which source and model reviewed each round.
func TestReadStickyReadsAFullFooter(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
	in.Source, in.Model, in.Unattended = "gadfly-review-pr@2.2.0", "anthropic/claude-opus-5.5", true
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>⛔ 1 blocking</code></summary>") ||
		!strings.Contains(earlier[0], "\n>\n> reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111) · via `gadfly-review-pr 2.2.0` · `anthropic/claude-opus-5.5` · unattended\n\n<!-- loupe digest=") {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
}

func TestReadStickyReadsCRLF(t *testing.T) {
	first := firstRound()
	lf, _, err := ReadSticky(first)
	if err != nil {
		t.Fatal(err)
	}
	crlf, _, err := ReadSticky(strings.ReplaceAll(first, "\n", "\r\n"))
	if err != nil || len(crlf) != 1 || crlf[0] != lf[0] {
		t.Fatalf("CRLF read differs: err %v", err)
	}
}

func TestReadStickyRefusesAnUnreadableBody(t *testing.T) {
	first := firstRound()
	earlier, _, err := ReadSticky(first)
	if err != nil {
		t.Fatal(err)
	}
	next := stickyInput(2, "bbbbbbb222", general("f-001", "question", false))
	next.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	second := Body(next)
	dropped := stickyInput(3, "ccccccc333", general("f-001", "question", false))
	dropped.Sticky = &StickyInput{Rounds: 3}
	third := Body(dropped)
	cases := map[string]string{
		"not sticky":      strings.Replace(first, " sticky=1", "", 1),
		"no marker":       first[:strings.Index(first, "<!-- loupe digest=")],
		"footer replaced": strings.Replace(first, "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)", "edited by hand", 1),
		// The footer is not carried into the collapsed round, so text added to it would be lost without a word.
		"footer extended": strings.Replace(first, "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)", "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111) keep this note", 1),
		"no divider":      strings.Replace(first, "\n\n---\n\nreviewed", "\n\nreviewed", 1),
		"text after meta": first + "\nA note added on GitHub.\n",
		// An earlier round whose delimiter was deleted on GitHub would otherwise vanish from the next edit.
		"round delimiter removed":    strings.Replace(second, "<!-- loupe-round -->\n\n", "", 1),
		"text before the rounds":     strings.Replace(second, "### Earlier rounds\n\n", "### Earlier rounds\n\nA note added on GitHub.\n\n", 1),
		"collapsed summary broken":   strings.Replace(second, "⛔ 1 blocking</code></summary>", "⛔ 1 blocking</code>", 1),
		"collapsed summary reworded": strings.Replace(second, "<summary>Round 1 · reviewed", "<summary>First pass · reviewed", 1),
		"block is not a disclosure":  strings.Replace(second, "<!-- loupe-round -->\n\n<details>", "<!-- loupe-round -->\n\nloose text\n\n<details>", 1),
		"sticky count too high":      strings.Replace(second, " sticky=2 -->", " sticky=3 -->", 1),
		"dropped count wrong":        strings.Replace(third, "The 2 oldest rounds", "The 5 oldest rounds", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := ReadSticky(body); err == nil {
				t.Fatalf("ReadSticky accepted:\n%s", body)
			}
		})
	}
}

func TestStickyRounds(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		rounds int
		sticky bool
	}{
		{"sticky body", firstRound(), 1, true},
		{"non-sticky body", Body(exampleInput()), 0, false},
		{"no marker", "no marker at all", 0, false},
		// A key edited on GitHub still marks the review as sticky, so it is read back and refused rather than skipped.
		{"sticky=0", strings.Replace(firstRound(), " sticky=1 -->", " sticky=0 -->", 1), 0, true},
		{"sticky=x", strings.Replace(firstRound(), " sticky=1 -->", " sticky=x -->", 1), 0, true},
		// A marker line appended on GitHub after loupe's own does not hide it.
		{"marker appended after", firstRound() + MetaPrefix + "v=1 round=9 -->\n", 1, true},
		{"key after sticky=", strings.Replace(firstRound(), " sticky=1 -->", " sticky=1 extra=x -->", 1), 1, true},
		{"key moved earlier", strings.Replace(strings.Replace(firstRound(), " sticky=1 -->", " -->", 1), " round=1 ", " sticky=1 round=1 ", 1), 1, true},
	}
	for _, c := range cases {
		if rounds, sticky := StickyRounds(c.body); rounds != c.rounds || sticky != c.sticky {
			t.Errorf("%s: StickyRounds = %d, %v; want %d, %v", c.name, rounds, sticky, c.rounds, c.sticky)
		}
	}
	if _, _, err := ReadSticky(firstRound() + MetaPrefix + "v=1 round=9 -->\n"); err == nil {
		t.Error("ReadSticky accepted a second marker appended after loupe's")
	}
	for _, tail := range []string{" sticky=0 -->", " sticky=x -->", " sticky=1 extra=x -->"} {
		if _, _, err := ReadSticky(strings.Replace(firstRound(), " sticky=1 -->", tail, 1)); err == nil {
			t.Errorf("ReadSticky accepted a marker ending %q", tail)
		}
	}
}

// Prose that opens on a divider and a heading, even loupe's own, is the human's and stays in the collapsed round.
func TestReadStickyKeepsProseThatLooksLikeASection(t *testing.T) {
	for prose, quoted := range map[string]string{
		"---\n\n### Notes\n\nKeep this.":         "> ---\n>\n> ### Notes\n>\n> Keep this.",
		"---\n\n### Must fix\n\nMy own heading.": "> ---\n>\n> ### Must fix\n>\n> My own heading.",
	} {
		in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
		in.Summary = prose
		in.Sticky = &StickyInput{Rounds: 1}
		earlier, _, err := ReadSticky(Body(in))
		if err != nil {
			t.Fatal(err)
		}
		want := "</summary>\n\n" + quoted + "\n>\n> ### Must fix\n>\n> <details>"
		if !strings.Contains(earlier[0], want) {
			t.Errorf("prose %q was cut:\n%s", prose, earlier[0])
		}
	}
}

// Every round carried into the next body MUST balance its disclosures, or text a person edited on GitHub would land
// outside its collapse. These are hand edits a person could make; each is refused rather than republished.
func TestReadStickyRefusesUnbalancedRounds(t *testing.T) {
	clean := stickyInput(1, "aaaaaaa111")
	clean.Summary = "Nothing to fix."
	clean.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(firstRound())
	if err != nil {
		t.Fatal(err)
	}
	next := stickyInput(2, "bbbbbbb222", general("f-002", "question", false))
	next.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	two := Body(next)
	droppedOnly := stickyInput(3, "ccccccc333")
	droppedOnly.Summary = "Nothing to fix."
	droppedOnly.Sticky = &StickyInput{Rounds: 3}
	cases := []struct{ name, body string }{
		// This layout writes every collapsed round with its footer, so one without it was edited on GitHub.
		{"footer removed from the newest collapsed round", strings.Replace(two, "\n>\n> reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)", "", 1)},
		// With every earlier round dropped nothing tells the layouts apart, so a second footer is refused.
		{"a second footer after dropped rounds", strings.Replace(Body(droppedOnly), "\n\n<!-- loupe digest=3", "\n\n---\n\nreviewed `ddddddd`\n\n<!-- loupe digest=3", 1)},
		{"stray </details> in a round with no findings", strings.Replace(Body(clean), "Nothing to fix.", "Nothing to fix.\n\n</details>\n\nOutside.", 1)},
		{"extra <details> in an earlier round", strings.Replace(two, "> ### Must fix\n", "> <details>\n> <summary>Mine</summary>\n>\n> ### Must fix\n", 1)},
		{"missing </details> in an earlier round", strings.Replace(two, "> Body f-001.\n>\n> </details>\n", "> Body f-001.\n", 1)},
		{"close tag after text in the shown round", strings.Replace(Body(clean), "Nothing to fix.", "Nothing to fix.</details>\n\nOutside.", 1)},
		{"round closed early, then a second disclosure", strings.Replace(two, "\n\n<!-- loupe digest=1", "\n\n</details>\n\nOutside.\n\n<details>\n<summary>Mine</summary>\n\n<!-- loupe digest=1", 1)},
		{"close tag after a span that spans lines", strings.Replace(Body(clean), "Nothing to fix.", "Nothing to fix. `a\nb` </details> `c`\n\nOutside.", 1)},
		{"comment left open before an earlier round's close", regexp.MustCompile(`(<!-- loupe digest=1[^\n]*-->\n\n)</details>`).ReplaceAllString(two, "$1<!--\n\n</details>")},
		// This loupe never writes a footer after the earlier rounds, and its newest collapsed round carries a footer,
		// so a body with both is a hand edit, not the v0.11.0 layout.
		{"a second footer after the earlier rounds", strings.Replace(two, "\n\n<!-- loupe digest=2", "\n\n---\n\nreviewed `ddddddd`\n\n<!-- loupe digest=2", 1)},
		// An open fence also hides the generated tail, so the structure checks refused this before the balance check.
		{"unclosed fence in the shown round", strings.Replace(two, "Body f-002.", "Body f-002.\n\n```", 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, _, err := ReadSticky(c.body); err == nil {
				t.Fatalf("ReadSticky accepted:\n%s", c.body)
			}
		})
	}
}

func TestReadStickyDropsTheRecordFromTheDemotedRound(t *testing.T) {
	earlier, _, err := ReadSticky(firstRound())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(earlier[0], recordPrefix) {
		t.Fatalf("the demoted round kept its record:\n%s", earlier[0])
	}
}

func TestStickyRoundsKeepOneRecordOnTheCurrentRound(t *testing.T) {
	body := firstRound()
	for round := 2; round <= 3; round++ {
		earlier, rounds, err := ReadSticky(body)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		in := stickyInput(round, strings.Repeat(string(rune('a'+round)), 10), general("f-00"+string(rune('0'+round)), "question", false))
		in.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
		body = Body(in)
	}
	if n := strings.Count(body, recordPrefix); n != 1 {
		t.Fatalf("a three-round sticky body holds %d records", n)
	}
	got, err := ReadRecord(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "f-003" {
		t.Fatalf("the record reads %+v, want the third round's f-003", got)
	}
}

func TestReadStickyReadsABodyWithoutARecord(t *testing.T) {
	body := firstRound()
	legacy := strings.Replace(body, recordOf(t, body)+"\n", "", 1)
	if _, rounds, err := ReadSticky(legacy); err != nil || rounds != 1 {
		t.Fatalf("a body written before the record: rounds %d, err %v", rounds, err)
	}
}

// A record anywhere but the tail was written on GitHub, and carrying it into a collapsed round would leave the next
// body with two, which no capture can read back.
func TestReadStickyRefusesARecordOutsideTheTail(t *testing.T) {
	body := firstRound()
	record := recordOf(t, body)
	edited := strings.Replace(body, "### Must fix", record+"\n\n### Must fix", 1)
	if _, _, err := ReadSticky(edited); err == nil || !strings.Contains(err.Error(), "findings record") {
		t.Fatalf("ReadSticky: %v, want a refusal naming the findings record", err)
	}
	fenced := strings.Replace(body, "### Must fix", "```\n"+record+"\n```\n\n### Must fix", 1)
	if _, _, err := ReadSticky(fenced); err != nil && strings.Contains(err.Error(), "findings record") {
		t.Fatalf("a fenced record was refused: %v", err)
	}
}

// A body in the v0.11.0 layout, footer after the earlier rounds and collapsed rounds without one, reads back and the
// next round writes the new layout. The round it showed keeps its footer, and the older round has none to recover.
func TestStickyMovesAV0110BodyToTheNewLayout(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sticky-v0.11.0-layout.md"))
	if err != nil {
		t.Fatal(err)
	}
	earlier, rounds, err := ReadSticky(string(data))
	if err != nil || rounds != 2 || len(earlier) != 2 {
		t.Fatalf("blocks %d rounds %d err %v", len(earlier), rounds, err)
	}
	if !strings.Contains(earlier[0], "\n>\n> reviewed `bbbbbbb` · via `gadfly-review-pr 2.2.0`\n\n<!-- loupe digest=2") ||
		strings.Contains(earlier[1], "reviewed `aaaaaaa`") {
		t.Fatalf("blocks:\n%s", strings.Join(earlier, "\n=====\n"))
	}
	next := stickyInput(3, "ccccccc333", general("f-001", "question", false))
	next.Source = "gadfly-review-pr@2.3.0"
	next.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	body := Body(next)
	indexes(t, body, "Body f-001.\n\n</details>\n\n---\n\nreviewed [`ccccccc`](https://github.com/o/r/commit/ccccccc333) · [changes since round 2](https://github.com/o/r/compare/bbbbbbb...ccccccc333) · via `gadfly-review-pr 2.3.0`\n\n---\n\n<!-- loupe-earlier -->",
		"<summary>Round 2 · reviewed <code>bbbbbbb</code>", "reviewed `bbbbbbb` · via `gadfly-review-pr 2.2.0`", "<summary>Round 1 · reviewed <code>aaaaaaa</code>")
	if _, err := ReadRecord(body); err != nil {
		t.Fatalf("ReadRecord: %v", err)
	}
	again, rounds, err := ReadSticky(body)
	if err != nil || rounds != 3 || len(again) != 3 || again[1] != earlier[0] || again[2] != earlier[1] {
		t.Fatalf("rounds %d err %v blocks:\n%s", rounds, err, strings.Join(again, "\n=====\n"))
	}
}

// Authored prose can end with a divider and a footer-shaped line. In a v0.11.0 body the footer at the tail is the
// round's own, so that prose is the round's text and reads back as it was written.
func TestStickyV0110BodyKeepsFooterShapedProse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sticky-v0.11.0-layout.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(data), "\n\n---\n\n<!-- loupe-earlier -->")
	if rest == "" || !strings.Contains(rest, " questions=1 ") {
		t.Fatalf("the fixture changed:\n%s", data)
	}
	prose := "Prose.\n\n---\n\nreviewed `abcdef`"
	body := prose + "\n\n---\n\n<!-- loupe-earlier -->" + strings.Replace(rest, " questions=1 ", " questions=0 ", 1)
	earlier, _, err := ReadSticky(body)
	if err != nil {
		t.Fatalf("ReadSticky: %v", err)
	}
	want := "<details>\n<summary>Round 2 · reviewed <code>bbbbbbb</code> · <code>🟢 no findings</code></summary>\n\n" +
		"> Prose.\n>\n> ---\n>\n> reviewed `abcdef`\n>\n> reviewed `bbbbbbb` · via `gadfly-review-pr 2.2.0`\n\n<!-- loupe digest=" + strings.Repeat("2", 64) +
		" publication=00000000-0000-4000-8000-000000000002 -->\n\n</details>"
	if earlier[0] != want {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
}

// A body in the format before 025 keeps its footer at the tail and in its collapsed rounds, like a new-layout body
// with a footer added on GitHub. When its round's prose also ends in a footer-shaped line it is refused with sticky,
// which the format allows, since only pre-release probe reviews carry that format.
func TestStickyPre025BodyWithFooterShapedProseIsRefused(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sticky-before-025-format.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(data), "\n\n---\n\n<!-- loupe-earlier -->")
	if rest == "" || !strings.Contains(rest, " questions=1 ") {
		t.Fatalf("the fixture changed:\n%s", data)
	}
	body := "Prose.\n\n---\n\nreviewed `abcdef`\n\n---\n\n<!-- loupe-earlier -->" + strings.Replace(rest, " questions=1 ", " questions=0 ", 1)
	if _, _, err := ReadSticky(body); err == nil {
		t.Fatalf("ReadSticky accepted:\n%s", body)
	}
	if _, _, err := ReadSticky(string(data)); err != nil {
		t.Fatalf("the fixture itself no longer reads back: %v", err)
	}
}

// The first build of the footer-under-its-round layout wrote no divider after a collapsed round's footer, and a review it
// published MUST still read back, carried as it is.
func TestReadStickyReadsARoundWithoutItsClosingDivider(t *testing.T) {
	body := fixture(t, "sticky-v0.12.0-layout.md")
	older := strings.Replace(body, "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n---\n\n<!-- loupe digest=1", "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n<!-- loupe digest=1", 1)
	if older == body {
		t.Fatal("the fixture has no closing divider to remove")
	}
	carried, rounds, err := ReadSticky(older)
	if err != nil || rounds != 3 || len(carried) != 3 {
		t.Fatalf("blocks %d rounds %d err %v", len(carried), rounds, err)
	}
	if !strings.Contains(carried[2], "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n<!-- loupe digest=1") {
		t.Fatalf("the older round was not carried as it is:\n%s", carried[2])
	}
}

// A clean round published before the no-findings chip opens on its prose, and it MUST still read back with that prose
// kept whole.
func TestReadStickyReadsACleanRoundWithoutItsChip(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111")
	in.Summary = "Nothing to fix."
	in.Sticky = &StickyInput{Rounds: 1}
	body := Body(in)
	older := strings.Replace(body, "`🟢 no findings`\n\n", "", 1)
	if older == body {
		t.Fatal("the fixture has no chip to remove")
	}
	earlier, rounds, err := ReadSticky(older)
	if err != nil || rounds != 1 || len(earlier) != 1 {
		t.Fatalf("blocks %d rounds %d err %v", len(earlier), rounds, err)
	}
	if !strings.Contains(earlier[0], "<code>🟢 no findings</code></summary>\n\n> Nothing to fix.\n>\n> reviewed ") {
		t.Fatalf("clean round read back wrong:\n%s", earlier[0])
	}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Every divider loupe wrote into the round goes, since the quote's edge marks the round and a rule inside it reads as a
// boundary between rounds. A divider the author wrote in the prose is theirs and stays.
func TestReadStickyQuotesTheDemotedRoundWithoutDividers(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true), general("f-002", "question", false))
	in.Summary = "Prose.\n\n---\n\nMore prose."
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	want := "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>⛔ 1 blocking</code> <code>🔵 1 question</code></summary>\n\n" +
		"> Prose.\n>\n> ---\n>\n> More prose.\n>\n" +
		"> ### Must fix\n>\n> <details>\n> <summary>⛔ <b>issue</b>: Title f-001</summary>\n>\n> Body f-001.\n>\n> </details>\n>\n" +
		"> ### Worth a look\n>\n> <details>\n> <summary>🔵 <b>question</b>: Title f-002</summary>\n>\n> Body f-002.\n>\n> </details>\n>\n" +
		"> reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n<!-- loupe digest=" + strings.Repeat("1", 64) +
		" publication=00000000-0000-4000-8000-000000000001 -->\n\n</details>"
	if earlier[0] != want {
		t.Fatalf("demoted round\n--- got ---\n%s\n--- want ---\n%s", earlier[0], want)
	}
}

// A round with no prose and no findings still has an edge: its footer alone is the quote.
func TestReadStickyQuotesAnEmptyRoundsFooter(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111")
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(earlier[0], "<code>🟢 no findings</code></summary>\n\n> reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)\n\n<!-- loupe digest=1") {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
	next := stickyInput(2, "bbbbbbb222")
	next.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	if _, _, err := ReadSticky(Body(next)); err != nil {
		t.Fatalf("the next round does not read back: %v", err)
	}
}

// A finding's location is a quote of its own, so inside the round's quote it nests one level deeper.
func TestReadStickyNestsALocationQuote(t *testing.T) {
	loc := Finding{ID: "f-001", Title: "Title f-001", Body: "Body f-001.", Label: "suggestion",
		Location: &Location{Path: "internal/a.go", Side: "RIGHT", Line: 12}}
	in := stickyInput(1, "aaaaaaa111", loc)
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(earlier[0], "\n>\n> > [`internal/a.go:12`](https://github.com/o/r/pull/7/files#diff-") {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
}

// A body in the v0.12.0 layout reads back after an upgrade. The round it showed is collapsed into a quote, and the
// rounds v0.12.0 collapsed are carried byte for byte apart from their number.
func TestStickyMovesAV0120BodyToTheQuotedLayout(t *testing.T) {
	old := fixture(t, "sticky-v0.12.0-layout.md")
	earlier, rounds, err := ReadSticky(old)
	if err != nil || rounds != 3 || len(earlier) != 3 {
		t.Fatalf("blocks %d rounds %d err %v", len(earlier), rounds, err)
	}
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 3 · reviewed <code>ccccccc</code> · <code>🟣 1 suggestion</code></summary>\n\n> One suggestion left.\n>\n> ### Worth a look\n") ||
		!strings.Contains(earlier[0], "\n>\n> > [`internal/a.go:12`](") ||
		!strings.HasSuffix(earlier[0], "\n>\n> reviewed [`ccccccc`](https://github.com/o/r/commit/ccccccc333) · [changes since round 2](https://github.com/o/r/compare/bbbbbbb222...ccccccc333) · via `gadfly-review-pr 2.3.0` · `anthropic/claude-opus-5.5`\n\n<!-- loupe digest="+strings.Repeat("3", 64)+" publication=00000000-0000-4000-8000-000000000003 -->\n\n</details>") {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
	blocks := strings.Split(old, "\n\n<!-- loupe-round -->\n\n")
	if len(blocks) != 3 {
		t.Fatalf("the fixture changed:\n%s", old)
	}
	for i, want := range []string{blocks[1], blocks[2][:strings.Index(blocks[2], "\n\n<!-- loupe digest=3")]} {
		if earlier[i+1] != want {
			t.Fatalf("round %d was not carried as it is\n--- got ---\n%s\n--- want ---\n%s", 2-i, earlier[i+1], want)
		}
	}
	next := stickyInput(4, "ddddddd444", general("f-001", "question", false))
	next.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	again, rounds, err := ReadSticky(Body(next))
	if err != nil || rounds != 4 || len(again) != 4 || again[1] != earlier[0] {
		t.Fatalf("rounds %d err %v blocks:\n%s", rounds, err, strings.Join(again, "\n=====\n"))
	}
	for i, block := range earlier[1:] {
		if want := roundNumber.ReplaceAllString(block, "<details>\n<summary>Round "+string(rune('2'-i))+" · "); again[i+2] != want {
			t.Fatalf("round %d changed:\n%s", 2-i, again[i+2])
		}
	}
}

// quotedThreeRounds is a sticky body whose two collapsed rounds were both quoted by this layout.
func quotedThreeRounds(t *testing.T) string {
	t.Helper()
	body := firstRound()
	for round := 2; round <= 3; round++ {
		earlier, rounds, err := ReadSticky(body)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		in := stickyInput(round, strings.Repeat(string(rune('a'+round-1)), 7)+strings.Repeat(string(rune('0'+round)), 3), general("f-001", "question", false))
		in.Summary = "Prose of round " + string(rune('0'+round)) + "."
		in.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
		body = Body(in)
	}
	if _, _, err := ReadSticky(body); err != nil {
		t.Fatalf("the three-round body does not read back: %v", err)
	}
	return body
}

// The newest collapsed round keeps its footer so the history shows each round's source and model, and a round that
// lost it was edited on GitHub.
func TestReadStickyRefusesAQuotedRoundThatLostItsFooter(t *testing.T) {
	body := quotedThreeRounds(t)
	cases := []struct{ name, old, new string }{
		{"newest round's quoted footer removed", "\n>\n> reviewed [`bbbbbbb`](https://github.com/o/r/commit/bbbbbbb222) · [changes since round 1](https://github.com/o/r/compare/aaaaaaa111...bbbbbbb222)\n", "\n"},
		{"newest round emptied", "> Prose of round 2.\n>\n> ### Worth a look\n>\n> <details>\n> <summary>🔵 <b>question</b>: Title f-001</summary>\n>\n> Body f-001.\n>\n> </details>\n>\n> reviewed [`bbbbbbb`](https://github.com/o/r/commit/bbbbbbb222) · [changes since round 1](https://github.com/o/r/compare/aaaaaaa111...bbbbbbb222)\n", "\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if strings.Count(body, c.old) != 1 {
				t.Fatalf("the body holds %q %d times", c.old, strings.Count(body, c.old))
			}
			edited := strings.Replace(body, c.old, c.new, 1)
			if _, _, err := ReadSticky(edited); err == nil {
				t.Fatalf("ReadSticky accepted:\n%s", edited)
			}
		})
	}
}

// A line that lost its quote marker on GitHub only renders outside the quote. Nothing in a round's content tells that
// edit from an earlier layout's prose, so the round is carried as it is rather than refused.
func TestReadStickyCarriesAQuotedRoundThatLostAMarker(t *testing.T) {
	body := quotedThreeRounds(t)
	cases := []struct{ name, old, new string }{
		{"prose", "\n> Prose of round 2.", "\nProse of round 2."},
		{"a marker without its space", "\n> Prose of round 2.", "\n>Prose of round 2."},
		{"a heading", "\n> ### Worth a look", "\n### Worth a look"},
		{"a finding line", "\n> Body f-001.\n>\n> </details>\n>\n> reviewed [`bbbbbbb`]", "\nBody f-001.\n>\n> </details>\n>\n> reviewed [`bbbbbbb`]"},
		{"an older round's line", "\n> ### Must fix", "\n### Must fix"},
		{"the newest round's footer", "\n> reviewed [`bbbbbbb`]", "\nreviewed [`bbbbbbb`]"},
		{"an older round's footer", "\n> reviewed [`aaaaaaa`]", "\nreviewed [`aaaaaaa`]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if strings.Count(body, c.old) != 1 {
				t.Fatalf("the body holds %q %d times", c.old, strings.Count(body, c.old))
			}
			earlier, _, err := ReadSticky(strings.Replace(body, c.old, c.new, 1))
			if err != nil {
				t.Fatalf("ReadSticky: %v", err)
			}
			if !strings.Contains(strings.Join(earlier, "\n"), c.new) {
				t.Fatalf("the edited line was not carried:\n%s", strings.Join(earlier, "\n"))
			}
		})
	}
}

// A round collapsed in an earlier layout MAY open on a quote the author wrote. It ends on a divider, not on a quoted
// footer, so it is not taken for a quoted round and its unquoted lines are carried as they are.
func TestReadStickyCarriesAnOlderRoundThatOpensOnAQuote(t *testing.T) {
	old := strings.Replace(fixture(t, "sticky-v0.12.0-layout.md"), "\n\nThe blocking issue is fixed; one question left.\n\n", "\n\n> Quoted by the author.\n\nThe blocking issue is fixed; one question left.\n\n", 1)
	earlier, _, err := ReadSticky(old)
	if err != nil {
		t.Fatalf("ReadSticky: %v", err)
	}
	if !strings.Contains(earlier[1], "</summary>\n\n> Quoted by the author.\n\nThe blocking issue is fixed; one question left.\n\n---\n\n#### Worth a look") {
		t.Fatalf("round 2:\n%s", earlier[1])
	}
}

// A round with no prose and no findings is quoted down to its footer, so that footer is the only line that can lose
// its marker, and the round is still carried.
func TestReadStickyCarriesAnEmptyQuotedRoundThatLostItsMarker(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111")
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, rounds, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	next := stickyInput(2, "bbbbbbb222")
	next.Summary = "Round two."
	next.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	body := Body(next)
	quoted := "\n\n> reviewed [`aaaaaaa`]"
	if strings.Count(body, quoted) != 1 {
		t.Fatalf("the empty round is not one quoted footer:\n%s", body)
	}
	carried, _, err := ReadSticky(strings.Replace(body, quoted, "\n\nreviewed [`aaaaaaa`]", 1))
	if err != nil {
		t.Fatalf("ReadSticky: %v", err)
	}
	if !strings.Contains(carried[1], "</summary>\n\nreviewed [`aaaaaaa`]") {
		t.Fatalf("the round was not carried as it was:\n%s", carried[1])
	}
}

// A round collapsed in the v0.11.0 layout carries no footer, so its prose MAY end in lines shaped like a quoted footer.
// It is carried as it is, since nothing but a hidden marker could tell it from a quoted round.
func TestReadStickyCarriesLegacyProseShapedLikeAQuotedFooter(t *testing.T) {
	findings := "### Must fix\n\n<details>\n<summary>⛔ <b>issue</b>: Title f-001</summary>\n\nBody f-001.\n\n</details>\n"
	for name, prose := range map[string]string{
		"a footer under a quote": "Nothing to fix.\n\n> note\nreviewed `abc1234`\n",
		"a quoted footer":        "Nothing to fix.\n\n> reviewed `abc1234`\n",
	} {
		t.Run(name, func(t *testing.T) {
			body := fixture(t, "sticky-v0.11.0-layout.md")
			if strings.Count(body, findings) != 1 {
				t.Fatal("the v0.11.0 fixture changed shape")
			}
			earlier, _, err := ReadSticky(strings.Replace(body, findings, prose, 1))
			if err != nil {
				t.Fatalf("ReadSticky: %v", err)
			}
			if !strings.Contains(earlier[len(earlier)-1], "</summary>\n\n"+prose) {
				t.Fatalf("the round was not carried as it was:\n%s", earlier[len(earlier)-1])
			}
		})
	}
}

// The terminal shows a collapsed round's rows with their pills as words, as it does for the round on top.
func TestPillsAsWordsReachesAQuotedRound(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", rated("f-001", "issue", "major", true))
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	next := stickyInput(2, "bbbbbbb222")
	next.Summary = "Fixed."
	next.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	shown := PillsAsWords(Body(next))
	if !strings.Contains(shown, "\n> <summary>⛔ <b>issue</b> MAJOR: Title f-001</summary>\n") || strings.Contains(shown, "<picture>") {
		t.Fatalf("pills left in the quoted round:\n%s", shown)
	}
}

const roundNote = "Pushed more commits? Add the `claude-review-requested` label for a fresh review of the whole PR."

// The note sits under the newest round's footer, where a reader finishes the round, and before the earlier rounds.
func TestStickyNoteFollowsTheNewestFooter(t *testing.T) {
	in := stickyGoldenInput(t)
	in.Sticky.Note = roundNote
	body := Body(in)
	want := "· via `gadfly-review-pr 2.2.0`\n\n<!-- loupe-note -->\n\n" + roundNote + "\n\n<!-- loupe-note-end -->\n\n---\n\n<!-- loupe-earlier -->"
	if !strings.Contains(body, want) {
		t.Fatalf("note not under the footer:\n%s", body)
	}
	if strings.Count(body, roundNote) != 1 {
		t.Fatalf("note appears %d times:\n%s", strings.Count(body, roundNote), body)
	}
	without := stickyGoldenInput(t)
	if Body(without) == body {
		t.Fatal("the note changed nothing")
	}
}

// A collapsed round keeps its summary but never its note, so a hint meant for the newest round does not repeat down
// the history.
func TestReadStickyDropsTheNoteFromTheDemotedRound(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
	in.Summary = "One blocking issue."
	noted := in
	noted.Sticky = &StickyInput{Rounds: 1, Note: roundNote}
	in.Sticky = &StickyInput{Rounds: 1}
	withNote, _, err := ReadSticky(Body(noted))
	if err != nil {
		t.Fatal(err)
	}
	plain, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(withNote[0], "claude-review-requested") || strings.Contains(withNote[0], "loupe-note") {
		t.Fatalf("the demoted round carries its note:\n%s", withNote[0])
	}
	if withNote[0] != plain[0] {
		t.Fatalf("a noted round must demote exactly as one without a note\n--- got ---\n%s\n--- want ---\n%s", withNote[0], plain[0])
	}
}

// Three rounds, each published with the note: only the newest carries it, and each collapsed round still reads back.
func TestStickyNoteShowsOnlyOnTheNewestRound(t *testing.T) {
	body := ""
	for round := 1; round <= 3; round++ {
		in := stickyInput(round, strings.Repeat(string(rune('a'+round-1)), 7)+"111", general("f-001", "issue", round == 1))
		in.Summary = "Round prose."
		in.Sticky = &StickyInput{Rounds: 1, Note: roundNote}
		if body != "" {
			earlier, rounds, err := ReadSticky(body)
			if err != nil {
				t.Fatalf("round %d: %v", round, err)
			}
			in.Sticky.Rounds, in.Sticky.Earlier = rounds+1, earlier
		}
		body = Body(in)
	}
	if n := strings.Count(body, roundNote); n != 1 {
		t.Fatalf("the note appears %d times, want once:\n%s", n, body)
	}
	if strings.Index(body, roundNote) > strings.Index(body, earlierDelimiter) {
		t.Fatalf("the note is not on the newest round:\n%s", body)
	}
	if _, _, err := ReadSticky(body); err != nil {
		t.Fatal(err)
	}
}

// A note is read by its delimiters alone, so a delimiter pair loupe did not write, or one left unpaired on GitHub, is
// refused rather than guessed at.
func TestReadStickyRefusesAMangledNote(t *testing.T) {
	in := stickyGoldenInput(t)
	in.Sticky.Note = roundNote
	body := Body(in)
	note := "\n\n" + noteStart + "\n\n" + roundNote + "\n\n" + noteEnd
	moved := strings.Replace(body, note, "", 1)
	moved = strings.Replace(moved, "### Earlier rounds\n", "### Earlier rounds"+note+"\n", 1)
	plain := strings.Replace(body, note, "", 1)
	intoProse := plain[:strings.Index(plain, "\n\n")] + note + plain[strings.Index(plain, "\n\n"):]
	for name, mangled := range map[string]string{
		"moved into the prose": intoProse,
		"no end":               strings.Replace(body, noteEnd+"\n", "", 1),
		"no start":             strings.Replace(body, noteStart+"\n", "", 1),
		"two notes":            strings.Replace(body, noteEnd, noteEnd+"\n\n"+noteStart+"\n\nMore.\n\n"+noteEnd, 1),
		"end first":            strings.NewReplacer(noteStart, noteEnd, noteEnd, noteStart).Replace(body),
		"moved":                moved,
	} {
		// Each is refused by the note's own checks, not by a later rule that a mangled body happens to break.
		if _, _, err := ReadSticky(mangled); err == nil || !strings.Contains(err.Error(), "round note") {
			t.Errorf("%s: got %v:\n%s", name, err, mangled)
		}
	}
}

// Authored text can quote a delimiter inside a fence, and that quote is not a note.
func TestReadStickyIgnoresAFencedNoteDelimiter(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
	in.Summary = "```\n" + noteStart + "\n```"
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(earlier[0], "> "+noteStart) {
		t.Fatalf("the fenced delimiter was dropped:\n%s", earlier[0])
	}
}
