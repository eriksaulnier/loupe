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

func TestStickyBodyHoldsEarlierRoundsBeforeTheFooter(t *testing.T) {
	earlier, rounds, err := ReadSticky(firstRound())
	if err != nil || rounds != 1 || len(earlier) != 1 {
		t.Fatalf("ReadSticky: %d blocks, rounds %d, err %v", len(earlier), rounds, err)
	}
	in := stickyInput(2, "bbbbbbb222", general("f-001", "question", false))
	in.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	body := Body(in)
	at := indexes(t, body, "### Worth a look", "\n\n---\n\n<!-- loupe-earlier -->\n\n### Earlier rounds\n\n<!-- loupe-round -->\n\n<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · ⛔ 1 blocking</summary>",
		"</details>\n\n---\n\nreviewed `bbbbbbb`", "sticky=2 -->")
	if strings.Contains(body, "reviewed `aaaaaaa`") {
		t.Fatalf("the earlier round kept its footer:\n%s", body)
	}
	if at[0] > at[1] || at[1] > at[2] || at[2] > at[3] {
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
	// The chips move into the summary and the footer goes, since the summary names the commit; the reconciliation
	// marker stays so an interrupted publish of this round still reconciles.
	want := "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · ⛔ 1 blocking</summary>\n\n" +
		"### Must fix\n\n<details>\n<summary>⛔ <b>issue</b>: Title f-001</summary>\n\nBody f-001.\n\n</details>\n\n" +
		"<!-- loupe digest=" + strings.Repeat("1", 64) + " publication=00000000-0000-4000-8000-000000000001 -->\n\n</details>"
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
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · 🔵 1 question</summary>\n\nProse stays.\n\n---\n\nAfter a break.\n\n---\n\n### Worth a look") {
		t.Fatalf("demoted round:\n%s", earlier[0])
	}
	next := stickyInput(6, "bbbbbbb222")
	next.Summary = "Nothing left."
	next.Sticky = &StickyInput{Rounds: 2, Earlier: earlier}
	again, _, err := ReadSticky(Body(next))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(again[0], "<details>\n<summary>Round 2 · reviewed <code>bbbbbbb</code> · no findings</summary>\n\nNothing left.\n\n<!-- loupe digest=") ||
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
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 2 · reviewed <code>bbbbbbb</code> · 🔵 1 question</summary>") ||
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
	if err != nil || rounds != 1 || len(earlier) != 1 || !strings.Contains(earlier[0], quoted) {
		t.Fatalf("rounds %d blocks %d err %v", rounds, len(earlier), err)
	}
	if got, sticky := StickyRounds(quoted); got != 0 || sticky {
		t.Fatalf("StickyRounds of a fenced marker = %d, want 0", got)
	}
}

// The footer can carry a source, a model and the unattended mark after the commit; only the commit reaches the summary.
func TestReadStickyReadsAFullFooter(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
	in.Source, in.Model, in.Unattended = "gadfly-review-pr@2.2.0", "anthropic/claude-opus-5.5", true
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(earlier[0], "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · ⛔ 1 blocking</summary>") ||
		strings.Contains(earlier[0], "claude-opus") || strings.Contains(earlier[0], "· unattended") {
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
		"footer replaced": strings.Replace(first, "reviewed `aaaaaaa`", "edited by hand", 1),
		// The footer is not carried into the collapsed round, so text added to it would be lost without a word.
		"footer extended": strings.Replace(first, "reviewed `aaaaaaa`", "reviewed `aaaaaaa` keep this note", 1),
		"no divider":      strings.Replace(first, "\n\n---\n\nreviewed", "\n\nreviewed", 1),
		"text after meta": first + "\nA note added on GitHub.\n",
		// An earlier round whose delimiter was deleted on GitHub would otherwise vanish from the next edit.
		"round delimiter removed":    strings.Replace(second, "<!-- loupe-round -->\n\n", "", 1),
		"text before the rounds":     strings.Replace(second, "### Earlier rounds\n\n", "### Earlier rounds\n\nA note added on GitHub.\n\n", 1),
		"collapsed summary broken":   strings.Replace(second, "⛔ 1 blocking</summary>", "⛔ 1 blocking", 1),
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
	for _, prose := range []string{"---\n\n### Notes\n\nKeep this.", "---\n\n### Must fix\n\nMy own heading."} {
		in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
		in.Summary = prose
		in.Sticky = &StickyInput{Rounds: 1}
		earlier, _, err := ReadSticky(Body(in))
		if err != nil {
			t.Fatal(err)
		}
		want := "</summary>\n\n" + prose + "\n\n---\n\n### Must fix\n\n<details>"
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
	cases := []struct{ name, body string }{
		{"stray </details> in a round with no findings", strings.Replace(Body(clean), "Nothing to fix.", "Nothing to fix.\n\n</details>\n\nOutside.", 1)},
		{"extra <details> in an earlier round", strings.Replace(two, "### Must fix\n", "<details>\n<summary>Mine</summary>\n\n### Must fix\n", 1)},
		{"missing </details> in an earlier round", strings.Replace(two, "Body f-001.\n\n</details>\n\n<!-- loupe digest=1", "Body f-001.\n\n<!-- loupe digest=1", 1)},
		{"close tag after text in the shown round", strings.Replace(Body(clean), "Nothing to fix.", "Nothing to fix.</details>\n\nOutside.", 1)},
		{"round closed early, then a second disclosure", strings.Replace(two, "\n\n<!-- loupe digest=1", "\n\n</details>\n\nOutside.\n\n<details>\n<summary>Mine</summary>\n\n<!-- loupe digest=1", 1)},
		{"close tag after a span that spans lines", strings.Replace(Body(clean), "Nothing to fix.", "Nothing to fix. `a\nb` </details> `c`\n\nOutside.", 1)},
		{"comment left open before an earlier round's close", regexp.MustCompile(`(<!-- loupe digest=1[^\n]*-->\n\n)</details>`).ReplaceAllString(two, "$1<!--\n\n</details>")},
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
