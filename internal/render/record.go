package render

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/findingid"
	"github.com/eriksaulnier/loupe/internal/markdown"
)

// RecordFinding is one published finding as the record holds it. Its JSON is publish.EnvelopeFinding's, so a round
// read back from GitHub and one read from a receipt have one shape.
type RecordFinding struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Location *RecordLocation `json:"location"`
	Label    string          `json:"label"`
	Blocking bool            `json:"blocking"`
}

type RecordLocation struct {
	Path      string `json:"path"`
	Side      string `json:"side"`
	Line      int    `json:"line"`
	StartLine int    `json:"startLine,omitempty"`
}

const (
	recordPrefix  = "<!-- loupe-findings "
	recordOmitted = recordPrefix + "v=1 omitted=length -->"
	// maxRecord bounds what a record may inflate to, since a review body is text anyone who can edit it controls.
	maxRecord = 16 << 20
)

var (
	ErrNoRecord      = errors.New("it carries no findings record")
	ErrRecordOmitted = errors.New("its findings record was left out to fit GitHub's length limit")

	recordFields = regexp.MustCompile(`^<!-- loupe-findings v=(\S+) sha256=([0-9a-f]{64}) (\S+) -->$`)
	recordAny    = regexp.MustCompile(`^<!-- loupe-findings v=(\S+) `)
	roundKey     = regexp.MustCompile(` round=([0-9]+)`)
)

// normalizeLines reads line endings as markdown.StructuralLines does, so the checksum is written and read over the
// same text whatever GitHub does to them.
func normalizeLines(s string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(s)
}

// recordData is the published findings in id order, as compact JSON deflated and then base64: an alphabet without
// '-' or '>', so no finding can close the comment that carries it.
func recordData(findings []Finding) string {
	sorted := slices.Clone(findings)
	slices.SortFunc(sorted, func(a, b Finding) int { return findingid.Compare(a.ID, b.ID) })
	out := make([]RecordFinding, 0, len(sorted))
	for _, f := range sorted {
		rf := RecordFinding{ID: f.ID, Title: f.Title, Body: f.Body, Label: f.Label, Blocking: f.Blocking}
		if f.Location != nil {
			rf.Location = &RecordLocation{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
		}
		out = append(out, rf)
	}
	var js bytes.Buffer
	enc := json.NewEncoder(&js)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		panic(fmt.Sprintf("encode findings record: %v", err))
	}
	var z bytes.Buffer
	w, err := flate.NewWriter(&z, flate.BestCompression)
	if err != nil {
		panic(fmt.Sprintf("deflate findings record: %v", err))
	}
	_, _ = w.Write(bytes.TrimSuffix(js.Bytes(), []byte("\n")))
	if err := w.Close(); err != nil {
		panic(fmt.Sprintf("deflate findings record: %v", err))
	}
	return base64.StdEncoding.EncodeToString(z.Bytes())
}

// withRecord puts the record line between head, which ends with the reconciliation marker, and meta. The checksum
// covers the rest of the body and the data, so an edit on GitHub to either is caught on read.
func withRecord(head, meta string, findings []Finding, omit bool) string {
	if omit {
		return head + recordOmitted + "\n" + meta
	}
	data := recordData(findings)
	return head + recordPrefix + "v=1 sha256=" + recordSum(head+meta, data) + " " + data + " -->\n" + meta
}

func recordSum(rest, data string) string {
	sum := sha256.Sum256([]byte(normalizeLines(rest) + "\n" + data))
	return hex.EncodeToString(sum[:])
}

// ReadRecord reads back the findings a body's record holds. Any doubt is an error and never a partial list, because a
// wrong previous round makes a reviewer drop an unfixed finding.
func ReadRecord(body string) ([]RecordFinding, error) {
	lines, structural := markdown.StructuralLines(body)
	at := -1
	for i, line := range lines {
		if !structural[i] || !strings.HasPrefix(line, recordPrefix) {
			continue
		}
		if at >= 0 {
			return nil, errors.New("it carries more than one findings record")
		}
		at = i
	}
	if at < 0 {
		return nil, ErrNoRecord
	}
	line := lines[at]
	if line == recordOmitted {
		return nil, ErrRecordOmitted
	}
	if m := recordAny.FindStringSubmatch(line); m != nil && m[1] != "1" {
		return nil, fmt.Errorf("its findings record is v=%s, which this loupe does not read", m[1])
	}
	m := recordFields.FindStringSubmatch(line)
	if m == nil {
		return nil, errors.New("its findings record line is not in loupe's form")
	}
	rest := strings.Join(slices.Delete(slices.Clone(lines), at, at+1), "\n")
	if recordSum(rest, m[3]) != m[2] {
		return nil, errors.New("it was changed on GitHub after loupe published it, so its findings record no longer matches")
	}
	raw, err := base64.StdEncoding.DecodeString(m[3])
	if err != nil {
		return nil, fmt.Errorf("its findings record is not base64: %w", err)
	}
	js, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), maxRecord+1))
	if err != nil {
		return nil, fmt.Errorf("its findings record does not inflate: %w", err)
	}
	if len(js) > maxRecord {
		return nil, errors.New("its findings record inflates past 16 MiB")
	}
	var findings []RecordFinding
	if trimmed := bytes.TrimSpace(js); len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New("its findings record is not a list of findings")
	}
	if err := json.Unmarshal(js, &findings); err != nil {
		return nil, fmt.Errorf("its findings record is not a list of findings: %w", err)
	}
	seen := map[string]bool{}
	for i, f := range findings {
		switch {
		case f.ID == "":
			return nil, fmt.Errorf("finding %d in its findings record has no id", i+1)
		case f.Title == "":
			return nil, fmt.Errorf("%s in its findings record has no title", f.ID)
		case seen[f.ID]:
			return nil, fmt.Errorf("its findings record holds %s twice", f.ID)
		}
		seen[f.ID] = true
	}
	return findings, nil
}

// RecordAsNote shows the record as a line a person can read, for a terminal that shows the body as raw Markdown. The
// record copies findings the body already shows, so the note names how many rather than repeating them.
func RecordAsNote(body string) string {
	lines, structural := markdown.StructuralLines(body)
	changed := false
	for i, line := range lines {
		m := recordFields.FindStringSubmatch(line)
		if !structural[i] || m == nil {
			continue
		}
		n, ok := recordCount(m[3])
		if !ok {
			continue
		}
		switch n {
		case 0:
			lines[i] = recordPrefix[:len(recordPrefix)-1] + ": a copy of no findings -->"
		case 1:
			lines[i] = recordPrefix[:len(recordPrefix)-1] + ": a copy of the 1 finding above -->"
		default:
			lines[i] = fmt.Sprintf("%s: a copy of the %d findings above -->", recordPrefix[:len(recordPrefix)-1], n)
		}
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

func recordCount(data string) (int, bool) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return 0, false
	}
	js, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), maxRecord+1))
	if err != nil || len(js) > maxRecord {
		return 0, false
	}
	var findings []json.RawMessage
	if json.Unmarshal(js, &findings) != nil {
		return 0, false
	}
	return len(findings), true
}

// MetaRound is the round= value on the marker line MetaSource reads, 0 when there is none.
func MetaRound(body string) int {
	line, ok := stickyMeta(body)
	if !ok {
		line, _ = lastMeta(body)
	}
	if m := roundKey.FindStringSubmatch(line); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}
