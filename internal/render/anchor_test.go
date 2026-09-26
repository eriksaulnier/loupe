package render

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/markdown"
)

func TestAnchorRoundTrips(t *testing.T) {
	text := "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>🟢 no findings</code></summary>\n\n> reviewed `aaaaaaa`\n\n</details>"
	line := sealAnchor("v=1 n=1 commit=aaaaaaa111", text)
	if !strings.HasPrefix(line, "<!-- loupe-round v=1 n=1 commit=aaaaaaa111 sha256=") || !strings.HasSuffix(line, " -->") {
		t.Fatalf("anchor line: %s", line)
	}
	a, err := parseAnchor(line)
	if err != nil {
		t.Fatal(err)
	}
	if a.n != 1 || a.commit != "aaaaaaa111" || a.fields != "v=1 n=1 commit=aaaaaaa111" || !a.matches(text) {
		t.Fatalf("parsed %+v", a)
	}
	if a.matches(text + " ") {
		t.Fatal("a changed round still matches")
	}
}

func TestAnchorChecksumReadsCRLFAsLFAndTrimsBlankLines(t *testing.T) {
	a, err := parseAnchor(sealAnchor("v=1 n=2 commit=bb", "one\n\ntwo"))
	if err != nil {
		t.Fatal(err)
	}
	if !a.matches("\n  \none\r\n\r\ntwo\n\n") {
		t.Fatal("line endings or blank ends changed the checksum")
	}
}

// The fields are checked as written, so a key a later loupe adds is covered by an older loupe's check.
func TestAnchorChecksumCoversEveryField(t *testing.T) {
	line := sealAnchor("v=1 n=2 commit=bb later=7", "text")
	for _, edit := range []struct{ old, new string }{{"n=2", "n=3"}, {"commit=bb", "commit=bc"}, {"later=7", "later=8"}} {
		a, err := parseAnchor(strings.Replace(line, edit.old, edit.new, 1))
		if err != nil {
			t.Fatal(err)
		}
		if a.matches("text") {
			t.Errorf("%s edited to %s still matches", edit.old, edit.new)
		}
	}
	a, _ := parseAnchor(line)
	if v, ok := a.number("later"); !ok || v != 7 {
		t.Fatalf("later=%d %v", v, ok)
	}
}

func TestAnchorRefusesAMalformedLine(t *testing.T) {
	sum := strings.Repeat("0", 64)
	for name, line := range map[string]string{
		"no fields":         "<!-- loupe-round sha256=" + sum + " -->",
		"no checksum":       "<!-- loupe-round v=1 n=1 commit=aa -->",
		"checksum not last": "<!-- loupe-round v=1 n=1 sha256=" + sum + " commit=aa -->",
		"short checksum":    "<!-- loupe-round v=1 n=1 commit=aa sha256=00 -->",
		"no version":        "<!-- loupe-round n=1 commit=aa sha256=" + sum + " -->",
		"version 2":         "<!-- loupe-round v=2 n=1 commit=aa sha256=" + sum + " -->",
		"no round":          "<!-- loupe-round v=1 commit=aa sha256=" + sum + " -->",
		"round zero":        "<!-- loupe-round v=1 n=0 commit=aa sha256=" + sum + " -->",
		"no commit":         "<!-- loupe-round v=1 n=1 sha256=" + sum + " -->",
		"commit not hex":    "<!-- loupe-round v=1 n=1 commit=xyz sha256=" + sum + " -->",
		"repeated key":      "<!-- loupe-round v=1 n=1 n=2 commit=aa sha256=" + sum + " -->",
		"upper case value":  "<!-- loupe-round v=1 n=1 commit=AA sha256=" + sum + " -->",
		"text after":        "<!-- loupe-round v=1 n=1 commit=aa sha256=" + sum + " --> x",
	} {
		if _, err := parseAnchor(line); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

// threeRounds is an anchored body holding three rounds, the newest with a note.
func threeRounds(t *testing.T) string {
	t.Helper()
	in := stickyThreeGoldenInput(t)
	in.Sticky.Note = "Push again for another round.\n\nOr ask."
	return Body(in)
}

func TestBodyAnchorsEveryRound(t *testing.T) {
	body := threeRounds(t)
	lines, structural := markdown.StructuralLines(body)
	var ns []int
	for i, line := range lines {
		if !strings.HasPrefix(line, anchorPrefix) {
			continue
		}
		a, err := parseAnchor(line)
		if err != nil || !structural[i] || lines[i+1] != "" {
			t.Fatalf("line %d: %v", i+1, err)
		}
		ns = append(ns, a.n)
	}
	if !strings.HasPrefix(body, "<!-- loupe-round v=1 n=3 commit=ccccccc333 blocking=1 issues=0 suggestions=1 questions=0 other=0 prose=1 note=3 sha256=") ||
		!slices.Equal(ns, []int{3, 2, 1}) {
		t.Fatalf("anchors %v:\n%s", ns, body)
	}
	if !strings.Contains(body, " · via `gadfly-review-pr 2.3.0`\n\nPush again for another round.\n\nOr ask.\n\n---\n\n<!-- loupe-earlier -->") {
		t.Fatalf("the note does not follow the footer:\n%s", body)
	}
	earlier, rounds, err := ReadSticky(body)
	if err != nil || rounds != 3 || len(earlier) != 3 {
		t.Fatalf("rounds %d, %d held, err %v", rounds, len(earlier), err)
	}
	for _, r := range earlier {
		if r.Edited {
			t.Errorf("round %d reads as edited", r.N)
		}
	}
	if strings.Contains(earlier[0].Block, "Push again") {
		t.Fatalf("the demoted round kept its note:\n%s", earlier[0].Block)
	}
}

// Opening prose written with blank lines around it is trimmed, so the anchor's line count holds.
func TestBodyTrimsTheProseItCounts(t *testing.T) {
	in := stickyInput(1, "aaaaaaa111", general("f-001", "issue", true))
	in.Summary = "\n\n  \nProse.\n  \n\n"
	in.Sticky = &StickyInput{Rounds: 1, Note: "\n \nNote.\n\n"}
	body := Body(in)
	if !strings.Contains(body, " prose=1 note=1 ") || !strings.Contains(body, "`⛔ 1 blocking`\n\nProse.\n\n---\n\n### Must fix") {
		t.Fatalf("prose not trimmed:\n%s", body)
	}
	if _, _, err := ReadSticky(body); err != nil {
		t.Fatal(err)
	}
}

// Read-back cuts the round on top by its anchor's counts, so a footer no pattern knows still demotes.
func TestReadStickyNeedsNoFooterPattern(t *testing.T) {
	body := firstRound()
	footer := "reviewed [`aaaaaaa`](https://github.com/o/r/commit/aaaaaaa111)"
	edited := strings.Replace(body, footer, footer+" · a segment no pattern knows", 1)
	edited = resealTop(t, edited)
	earlier, _, err := ReadSticky(edited)
	if err != nil {
		t.Fatal(err)
	}
	if earlier[0].Edited || !strings.Contains(earlier[0].Block, "> "+footer+" · a segment no pattern knows\n\n<!-- loupe digest=") ||
		strings.Contains(earlier[0].Block, "`⛔") {
		t.Fatalf("demoted round:\n%s", earlier[0].Block)
	}
}

// resealTop writes the top anchor again over an edited body, as a later loupe that changed the layout would write it.
func resealTop(t *testing.T, body string) string {
	t.Helper()
	top, rest, _ := strings.Cut(body, "\n\n")
	a, err := parseAnchor(top)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(rest, "\n")
	end := slices.Index(lines, earlierDelimiter) - 3
	digest := len(lines) - 1
	for !digestLine.MatchString(lines[digest]) {
		digest--
	}
	if end < 0 {
		end = digest - 1
	}
	text := strings.Join(lines[:end], "\n") + "\n" + lines[digest]
	return sealAnchor(a.fields, text) + "\n\n" + rest
}

// Rounds already anchored are carried byte for byte, anchor and number included.
func TestReadStickyCarriesAnchoredRoundsAsTheyAre(t *testing.T) {
	earlier, rounds, err := ReadSticky(threeRounds(t))
	if err != nil {
		t.Fatal(err)
	}
	next := stickyInput(4, "ddddddd444")
	next.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
	again, _, err := ReadSticky(Body(next))
	if err != nil || !slices.Equal(again[1:], earlier) {
		t.Fatalf("err %v, carried:\n%s", err, strings.Join(blocks(again), "\n=====\n"))
	}
}

func TestReadStickyRefusesABrokenAnchoredBody(t *testing.T) {
	body := threeRounds(t)
	top, _, _ := strings.Cut(body, "\n")
	anchors := regexp.MustCompile(`(?m)^<!-- loupe-round v=1 n=([12]) commit=[0-9a-f]+ sha256=[0-9a-f]{64} -->$`).FindAllString(body, -1)
	if len(anchors) != 2 {
		t.Fatalf("anchors %v", anchors)
	}
	dropped := stickyInput(3, "ccccccc333")
	dropped.Sticky = &StickyInput{Rounds: 3}
	droppedAll := Body(dropped)
	cases := map[string]string{
		"malformed top anchor":         strings.Replace(body, " n=3 ", " n=three ", 1),
		"top anchor at version 2":      strings.Replace(body, "v=1 n=3", "v=2 n=3", 1),
		"top anchor without counts":    strings.Replace(body, " prose=1 note=3", "", 1),
		"top round misnumbered":        strings.Replace(body, " sticky=3 -->", " sticky=4 -->", 1),
		"collapsed round misnumbered":  strings.Replace(body, anchors[1], strings.Replace(anchors[1], " n=1 ", " n=5 ", 1), 1),
		"collapsed round removed":      strings.Replace(body, anchors[0], "", 1),
		"malformed collapsed anchor":   strings.Replace(body, anchors[1], strings.Replace(anchors[1], "commit=", "commit=x", 1), 1),
		"legacy delimiter mixed in":    strings.Replace(body, anchors[1], "<!-- loupe-round -->", 1),
		"no top anchor":                strings.Replace(body, top+"\n", "", 1),
		"top anchor after the rounds":  strings.Replace(strings.Replace(body, top+"\n", "", 1), "### Earlier rounds", "### Earlier rounds\n\n"+top, 1),
		"anchor inside the top round":  strings.Replace(body, "### Must fix", anchors[1]+"\n\n### Must fix", 1),
		"text before the rounds":       strings.Replace(body, "### Earlier rounds\n\n", "### Earlier rounds\n\nA note added on GitHub.\n\n", 1),
		"no divider before the rounds": strings.Replace(body, "\n\n---\n\n<!-- loupe-earlier -->", "\n\n<!-- loupe-earlier -->", 1),
		"dropped count wrong":          strings.Replace(droppedAll, "The 2 oldest rounds", "The 5 oldest rounds", 1),
		"earlier section with nothing": strings.Replace(Body(stickyInputOne()), "\n\n<!-- loupe digest=", "\n\n---\n\n<!-- loupe-earlier -->\n\n### Earlier rounds\n\n<!-- loupe digest=", 1),
		"two earlier sections":         strings.Replace(body, "### Earlier rounds", "### Earlier rounds\n\n<!-- loupe-earlier -->", 1),
		"not sticky":                   strings.Replace(body, " sticky=3", "", 1),
		"record outside the tail":      strings.Replace(body, "### Must fix", recordOf(t, body)+"\n\n### Must fix", 1),
		"text after the rounds":        strings.Replace(body, "\n\n"+lastDigest(body), "\n\nAdded on GitHub.\n\n"+lastDigest(body), 1),
	}
	if _, err := parseAnchor(top); err != nil {
		t.Fatal(err)
	}
	for name, broken := range cases {
		t.Run(name, func(t *testing.T) {
			if broken == body {
				t.Fatal("the edit changed nothing")
			}
			if _, _, err := ReadSticky(broken); err == nil {
				t.Fatalf("ReadSticky accepted:\n%s", broken)
			}
		})
	}
}

func stickyInputOne() Input {
	in := stickyInput(1, "aaaaaaa111")
	in.Sticky = &StickyInput{Rounds: 1}
	return in
}

func lastDigest(body string) string {
	all := regexp.MustCompile(`(?m)^<!-- loupe digest=\S+ publication=\S+ -->$`).FindAllString(body, -1)
	return all[len(all)-1]
}

// An anchor quoted inside a fence is text, as every other marker is.
func TestReadStickyIgnoresAFencedAnchor(t *testing.T) {
	quoted := "```\n" + sealAnchor("v=1 n=9 commit=ab", "x") + "\n```"
	in := stickyInput(1, "aaaaaaa111", Finding{ID: "f-001", Title: "Quotes an anchor", Body: quoted, General: true})
	in.Sticky = &StickyInput{Rounds: 1}
	earlier, _, err := ReadSticky(Body(in))
	if err != nil || earlier[0].Edited || !strings.Contains(earlier[0].Block, "> "+strings.ReplaceAll(quoted, "\n", "\n> ")) {
		t.Fatalf("err %v:\n%s", err, earlier[0].Block)
	}
}

// A collapsed round edited on GitHub is carried as found, and named once: its anchor is written again over what was
// found, so the next read finds loupe's own bytes.
func TestReadStickyCarriesAnEditedCollapsedRound(t *testing.T) {
	body := threeRounds(t)
	edited := strings.Replace(body, "> Body f-001.\n>\n> </details>\n>\n> reviewed [`aaaaaaa`]", "> Body f-001, fixed a typo.\n>\n> </details>\n>\n> reviewed [`aaaaaaa`]", 1)
	if edited == body {
		t.Fatal("the edit changed nothing")
	}
	earlier, _, err := ReadSticky(edited)
	if err != nil {
		t.Fatal(err)
	}
	r := earlier[2]
	if !r.Edited || r.N != 1 || !strings.Contains(r.Block, "fixed a typo") || earlier[1].Edited || earlier[0].Edited {
		t.Fatalf("rounds: %+v", earlier)
	}
	if !strings.HasPrefix(r.Anchor, "<!-- loupe-round v=1 n=1 commit=aaaaaaa111 sha256=") || strings.Contains(edited, r.Anchor) {
		t.Fatalf("anchor not written again: %s", r.Anchor)
	}
	next := stickyInput(4, "ddddddd444")
	next.Sticky = &StickyInput{Rounds: 4, Earlier: earlier}
	again, _, err := ReadSticky(Body(next))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range again {
		if r.Edited {
			t.Fatalf("round %d is named as edited a second time", r.N)
		}
	}
	if again[3].Block != r.Block {
		t.Fatalf("the edited round was not carried byte for byte:\n%s", again[3].Block)
	}
}

// The round on top edited on GitHub no longer fits its anchor's counts, so it is collapsed whole: its chips row, its
// dividers and its note are quoted as found.
func TestReadStickyCollapsesAnEditedTopRoundWhole(t *testing.T) {
	body := threeRounds(t)
	edited := strings.Replace(body, "A new blocking issue, and one suggestion.", "A new blocking issue, and one suggestion. Edited.", 1)
	earlier, _, err := ReadSticky(edited)
	if err != nil {
		t.Fatal(err)
	}
	top := earlier[0]
	if !top.Edited || earlier[1].Edited {
		t.Fatalf("rounds: %+v", earlier)
	}
	for _, want := range []string{
		"<details>\n<summary>Round 3 · reviewed <code>ccccccc</code> · <code>⛔ 1 blocking</code> <code>🟣 1 suggestion</code></summary>\n\n> `⛔ 1 blocking` `🟣 1 suggestion`\n>\n> A new blocking issue, and one suggestion. Edited.\n>\n> ---\n>\n> ### Must fix\n",
		"> ---\n>\n> reviewed [`ccccccc`]",
		"\n>\n> Push again for another round.\n>\n> Or ask.\n\n<!-- loupe digest=3",
	} {
		if !strings.Contains(top.Block, want) {
			t.Fatalf("missing %q:\n%s", want, top.Block)
		}
	}
	a, err := parseAnchor(top.Anchor)
	if err != nil || !a.matches(top.Block) || a.fields != "v=1 n=3 commit=ccccccc333" {
		t.Fatalf("anchor %s: %v", top.Anchor, err)
	}
}

// An edit is carried, but never one that lets its text escape its round's collapse.
func TestReadStickyRefusesAnEditedRoundThatIsNotOneDisclosure(t *testing.T) {
	body := threeRounds(t)
	for name, edited := range map[string]string{
		"collapsed round": strings.Replace(body, "\n\n<!-- loupe digest=1", "\n\n</details>\n\nOutside.\n\n<details>\n<summary>Mine</summary>\n\n<!-- loupe digest=1", 1),
		"top round":       strings.Replace(body, "A new blocking issue, and one suggestion.", "A new blocking issue.\n\n</details>", 1),
	} {
		if _, _, err := ReadSticky(edited); err == nil || !strings.Contains(err.Error(), "disclosure") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Each earlier layout reads back through the legacy reader once. The body composed from it is anchored throughout, and
// the rounds it holds are then carried byte for byte.
func TestEveryLegacyLayoutMovesToAnchors(t *testing.T) {
	for _, name := range []string{"sticky-before-025-format.md", "sticky-v0.11.0-layout.md", "sticky-v0.12.0-layout.md", "sticky-v0.13.0-layout.md"} {
		t.Run(name, func(t *testing.T) {
			old := fixture(t, name)
			if strings.Contains(old, anchorPrefix+"v=") {
				t.Fatal("the fixture carries an anchor")
			}
			earlier, rounds, err := ReadSticky(old)
			if err != nil {
				t.Fatal(err)
			}
			for i, r := range earlier {
				a, err := parseAnchor(r.Anchor)
				if err != nil || a.n != rounds-i || !a.matches(r.Block) || r.Edited ||
					!strings.Contains(r.Block, "<summary>Round "+strconv.Itoa(a.n)+" · reviewed <code>"+shortSHA(a.commit)+"</code>") {
					t.Fatalf("round %d: %+v, %v", i, r, err)
				}
			}
			if strings.Contains(earlier[0].Block, "loupe-note") || strings.Contains(earlier[0].Block, "Pushed more commits") {
				t.Fatalf("the demoted round kept its note:\n%s", earlier[0].Block)
			}
			next := stickyInput(rounds+1, "eeeeeee555", general("f-001", "question", false))
			next.Sticky = &StickyInput{Rounds: rounds + 1, Earlier: earlier}
			body := Body(next)
			if strings.Contains(body, roundDelimiter) {
				t.Fatalf("the composed body keeps a legacy delimiter:\n%s", body)
			}
			again, _, err := ReadSticky(body)
			if err != nil || !slices.Equal(again[1:], earlier) {
				t.Fatalf("err %v, carried:\n%s", err, strings.Join(blocks(again), "\n=====\n"))
			}
		})
	}
}

// The terminal shows each anchor as its round's number, since its checksum means nothing to a reader.
func TestAnchorsAsNotes(t *testing.T) {
	body := threeRounds(t)
	shown := AnchorsAsNotes(body)
	for _, n := range []string{"3", "2", "1"} {
		if strings.Count(shown, "<!-- loupe-round "+n+" -->") != 1 {
			t.Fatalf("round %s not shown:\n%s", n, shown)
		}
	}
	if strings.Contains(shown, "sha256=") && !strings.Contains(shown, "loupe-findings") || strings.Contains(shown, "commit=") {
		t.Fatalf("an anchor is left:\n%s", shown)
	}
	fenced := "```\n" + sealAnchor("v=1 n=9 commit=ab", "x") + "\n```"
	if AnchorsAsNotes(fenced) != fenced {
		t.Fatal("a fenced anchor was rewritten")
	}
	if AnchorsAsNotes(shown) != shown {
		t.Fatal("rewriting twice changed the text")
	}
}

// The top anchor is line 1 of the body. Anything above it was added on GitHub, so the round on top is carried as found
// and named, prefix included, rather than read as unedited or refused.
func TestReadStickyCarriesTextAboveTheTopAnchor(t *testing.T) {
	body := threeRounds(t)
	for name, prefix := range map[string]string{"a blank line": "\n", "a whitespace line": "  \n", "text": "Added on GitHub.\n\n"} {
		t.Run(name, func(t *testing.T) {
			earlier, _, err := ReadSticky(prefix + body)
			if err != nil {
				t.Fatal(err)
			}
			top := earlier[0]
			if !top.Edited || top.N != 3 || earlier[1].Edited || earlier[2].Edited {
				t.Fatalf("rounds: %+v", earlier)
			}
			if text := strings.TrimSpace(prefix); text != "" && !strings.Contains(top.Block, "</summary>\n\n> "+text+"\n>\n") {
				t.Fatalf("the prefix was not carried:\n%s", top.Block)
			}
			if !strings.Contains(top.Block, "> `⛔ 1 blocking` `🟣 1 suggestion`\n") || strings.Contains(top.Block, anchorPrefix) {
				t.Fatalf("the round was not quoted whole, or kept its anchor:\n%s", top.Block)
			}
			next := stickyInput(4, "ddddddd444")
			next.Sticky = &StickyInput{Rounds: 4, Earlier: earlier}
			again, _, err := ReadSticky(Body(next))
			if err != nil || again[1] != (Round{N: 3, Commit: top.Commit, Anchor: top.Anchor, Block: top.Block}) {
				t.Fatalf("err %v, round 3 read back as %+v", err, again[1])
			}
		})
	}
}
