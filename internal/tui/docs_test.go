package tui

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var docsKey = regexp.MustCompile("`([^`]+)`")

// TestKeyTableMatchesHelp pins the key table in docs/review-workflow.md to the finding detail's help on a pull request run: the table names
// only keys review binds, and every letter key of the detail view is in it.
func TestKeyTableMatchesHelp(t *testing.T) {
	documented := docsKeyTable(t, "\n### 4. You decide each finding\n")

	bound := map[string]bool{}
	for _, k := range append((publishUI{}).helpSections(GlyphSet{})[viewDetail].keys, everywhere.keys...) {
		for key := range strings.SplitSeq(k.Keys, " / ") {
			bound[key] = true
		}
	}
	for key := range documented {
		if !bound[key] {
			t.Errorf("docs/review-workflow.md names key %q, which the finding detail's help does not", key)
		}
	}
	for _, k := range (publishUI{}).helpSections(GlyphSet{})[viewDetail].keys {
		for key := range strings.SplitSeq(k.Keys, " / ") {
			if r, _ := utf8.DecodeRuneInString(key); utf8.RuneCountInString(key) == 1 && unicode.IsLetter(r) && !documented[key] {
				t.Errorf("the finding detail binds %q (%s), which the key table in docs/review-workflow.md MUST list", key, k.Verb)
			}
		}
	}
}

// docsKeyTable is the set of keys in the first column of the first table after marker in docs/review-workflow.md.
func docsKeyTable(t *testing.T, marker string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile("../../docs/review-workflow.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(data), marker)
	if !ok {
		t.Fatalf("no %q in docs/review-workflow.md", strings.TrimSpace(marker))
	}
	documented := map[string]bool{}
	inTable := false
	for line := range strings.SplitSeq(after, "\n") {
		if !strings.HasPrefix(line, "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		first, _, _ := strings.Cut(strings.TrimPrefix(line, "|"), "|")
		for _, m := range docsKey.FindAllStringSubmatch(first, -1) {
			documented[m[1]] = true
		}
	}
	if len(documented) == 0 {
		t.Fatalf("no key table after %q in docs/review-workflow.md", strings.TrimSpace(marker))
	}
	return documented
}

// TestApplyKeyTableMatchesHelp pins the local-run key table in docs/review-workflow.md to applyUI's help: the table
// names only keys a local run binds, every letter key of its detail and list help is in it, and e and o are not.
func TestApplyKeyTableMatchesHelp(t *testing.T) {
	documented := docsKeyTable(t, "\nOn a local run, ")
	sections := (applyUI{}).helpSections(GlyphSet{})
	bound := map[string]bool{}
	var letters []helpKey
	for _, k := range slices.Concat(sections[viewDetail].keys, sections[viewList].keys, everywhere.keys) {
		// An alias such as "[/], n for next" names keys between its slashes and commas, each before its words.
		for _, keys := range []string{k.Keys, k.Alias} {
			for part := range strings.SplitSeq(strings.ReplaceAll(strings.ReplaceAll(keys, " / ", "/"), ", ", "/"), "/") {
				if fields := strings.Fields(part); len(fields) > 0 {
					bound[fields[0]] = true
				}
			}
		}
	}
	for _, k := range slices.Concat(sections[viewDetail].keys, sections[viewList].keys) {
		for key := range strings.SplitSeq(k.Keys, " / ") {
			if r, _ := utf8.DecodeRuneInString(key); utf8.RuneCountInString(key) == 1 && unicode.IsLetter(r) {
				letters = append(letters, helpKey{Keys: key, Verb: k.Verb})
			}
		}
	}
	for key := range documented {
		if !bound[key] {
			t.Errorf("the local-run table names key %q, which a local run's help does not", key)
		}
	}
	for _, k := range letters {
		if !documented[k.Keys] {
			t.Errorf("a local run binds %q (%s), which the local-run table MUST list", k.Keys, k.Verb)
		}
	}
	for _, absent := range []string{"e", "o"} {
		if documented[absent] {
			t.Errorf("the local-run table names %q, which a local run does not bind", absent)
		}
	}
}
