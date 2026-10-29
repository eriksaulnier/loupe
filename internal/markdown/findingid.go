package markdown

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const ruleFindingID = "finding-id"

const (
	idFix      = "name the earlier finding by its subject, or put a codebase's own identifier in a fence, then "
	titleIDFix = "name the earlier finding by its subject, or put a codebase's own identifier in a fence in the body or impact and word the title around it, then "
)

// FindingIDs refuses text that cites a finding id outside a fence or a URL. Every round numbers its findings from
// f-001, so an id read from an earlier round names another finding in this one, and the review's reader can resolve
// neither. Fences are read as Check reads them, so text should pass Check first. retry ends the fix.
func FindingIDs(text string, kind Kind, retry string) error {
	lines := splitLines(text)
	var err error
	walkLines(lines, func(i int, _ string, _ bool) {
		if err == nil {
			if match := findingID(lines[i]); match != "" {
				err = refuseID(kind.field(), match, i+1, idFix+retry)
			}
		}
	})
	return err
}

// TitleFindingIDs is FindingIDs for a title, which is one escaped paragraph: a backtick run there is text, not a
// fence, so nothing in a title is exempt but a URL.
func TitleFindingIDs(title, retry string) error {
	for i, line := range splitLines(title) {
		if match := findingID(line); match != "" {
			return refuseID("title", match, i+1, titleIDFix+retry)
		}
	}
	return nil
}

func refuseID(field, match string, line int, fix string) error {
	r := refusal.New(refusal.Markdown, fmt.Sprintf("%s cites finding id %s at line %d; a reader of the review cannot resolve it, since every round numbers its findings from f-001", field, match, line), fix)
	r.Details = map[string]any{"rule": ruleFindingID, "field": field, "match": match, "line": line}
	return r
}

// findingID returns the first f- followed by three or more ASCII digits that no letter, digit or _ touches, outside
// a URL.
func findingID(line string) string {
	line = blankURLs(line)
	for i := 0; i+1 < len(line); i++ {
		if line[i] != 'f' || line[i+1] != '-' || (i > 0 && isWordByte(line[i-1])) {
			continue
		}
		end := i + 2
		for end < len(line) && line[end] >= '0' && line[end] <= '9' {
			end++
		}
		if end-i-2 >= 3 && (end == len(line) || !isWordByte(line[end])) {
			return line[i:end]
		}
	}
	return ""
}

// blankURLs replaces each run from http:// or https:// to the next whitespace or > with spaces.
func blankURLs(line string) string {
	out := []byte(line)
	for i := 0; i < len(line); i++ {
		rest := line[i:]
		if !strings.HasPrefix(rest, "http://") && !strings.HasPrefix(rest, "https://") {
			continue
		}
		end := strings.IndexFunc(rest, func(r rune) bool { return unicode.IsSpace(r) || r == '>' })
		if end < 0 {
			end = len(rest)
		}
		for k := i; k < i+end; k++ {
			out[k] = ' '
		}
		i += end
	}
	return string(out)
}

func isWordByte(c byte) bool { return isLetter(c) || (c >= '0' && c <= '9') || c == '_' }
