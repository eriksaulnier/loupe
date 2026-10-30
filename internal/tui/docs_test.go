package tui

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

var docsKey = regexp.MustCompile("`([^`]+)`")

// TestKeyTableMatchesHelp pins the key table in docs/review-workflow.md to the finding detail's help: the table names
// only keys review binds, and every letter key of the detail view is in it.
func TestKeyTableMatchesHelp(t *testing.T) {
	data, err := os.ReadFile("../../docs/review-workflow.md")
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(data), "\n### 4. You decide each finding\n")
	if !ok {
		t.Fatal("no section 4 in docs/review-workflow.md")
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
		t.Fatal("no key table in section 4 of docs/review-workflow.md")
	}

	bound := map[string]bool{}
	for _, k := range append((&Model{}).helpSections()[viewDetail].keys, everywhere.keys...) {
		for key := range strings.SplitSeq(k.Keys, " / ") {
			bound[key] = true
		}
	}
	for key := range documented {
		if !bound[key] {
			t.Errorf("docs/review-workflow.md names key %q, which the finding detail's help does not", key)
		}
	}
	for _, k := range (&Model{}).helpSections()[viewDetail].keys {
		for key := range strings.SplitSeq(k.Keys, " / ") {
			if r, _ := utf8.DecodeRuneInString(key); utf8.RuneCountInString(key) == 1 && unicode.IsLetter(r) && !documented[key] {
				t.Errorf("the finding detail binds %q (%s), which the key table in docs/review-workflow.md MUST list", key, k.Verb)
			}
		}
	}
}
