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

// RecordAssessment is draft.Assessment as the record holds it: the round's status for an earlier finding.
type RecordAssessment struct {
	Ref     string        `json:"ref"`
	Status  string        `json:"status"`
	Finding RecordEarlier `json:"finding"`
}

type RecordEarlier struct {
	RecordFinding
	FiledIn RecordFiledIn `json:"filedIn"`
}

type RecordFiledIn struct {
	Round     int    `json:"round"`
	ReviewURL string `json:"reviewUrl"`
	Commit    string `json:"commit,omitempty"`
}

// recordV2 is version 2's data. A round that assessed nothing writes version 1's bare list, so its body keeps every
// byte an older loupe wrote and an older loupe can still read it back.
type recordV2 struct {
	Findings    []RecordFinding    `json:"findings"`
	Assessments []RecordAssessment `json:"assessments"`
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

// recordData is the published findings in id order, and any assessments, as compact JSON deflated and then base64: an
// alphabet without '-' or '>', so no finding can close the comment that carries it.
func recordData(findings []Finding, assessments []RecordAssessment) (version, data string) {
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
	var payload any = out
	version = "1"
	if len(assessments) > 0 {
		payload, version = recordV2{Findings: out, Assessments: assessments}, "2"
	}
	var js bytes.Buffer
	enc := json.NewEncoder(&js)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
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
	return version, base64.StdEncoding.EncodeToString(z.Bytes())
}

// withRecord puts the record line between head, which ends with the reconciliation marker, and meta. The checksum
// covers the rest of the body and the data, so an edit on GitHub to either is caught on read.
func withRecord(head, meta string, findings []Finding, assessments []RecordAssessment, omit bool) string {
	if omit {
		return head + recordOmitted + "\n" + meta
	}
	version, data := recordData(findings, assessments)
	return head + recordPrefix + "v=" + version + " sha256=" + recordSum(head+meta, data) + " " + data + " -->\n" + meta
}

func recordSum(rest, data string) string {
	sum := sha256.Sum256([]byte(normalizeLines(rest) + "\n" + data))
	return hex.EncodeToString(sum[:])
}

// ReadRecord reads back the findings and assessments a body's record holds. Any doubt is an error and never a partial
// list, because a wrong previous round makes a reviewer drop an unfixed finding.
func ReadRecord(body string) ([]RecordFinding, []RecordAssessment, error) {
	lines, structural := markdown.StructuralLines(body)
	at := -1
	for i, line := range lines {
		if !structural[i] || !strings.HasPrefix(line, recordPrefix) {
			continue
		}
		if at >= 0 {
			return nil, nil, errors.New("it carries more than one findings record")
		}
		at = i
	}
	if at < 0 {
		return nil, nil, ErrNoRecord
	}
	line := lines[at]
	if line == recordOmitted {
		return nil, nil, ErrRecordOmitted
	}
	if m := recordAny.FindStringSubmatch(line); m != nil && m[1] != "1" && m[1] != "2" {
		return nil, nil, fmt.Errorf("its findings record is v=%s, which this loupe does not read", m[1])
	}
	m := recordFields.FindStringSubmatch(line)
	if m == nil {
		return nil, nil, errors.New("its findings record line is not in loupe's form")
	}
	rest := strings.Join(slices.Delete(slices.Clone(lines), at, at+1), "\n")
	if recordSum(rest, m[3]) != m[2] {
		return nil, nil, errors.New("it was changed on GitHub after loupe published it, so its findings record no longer matches")
	}
	raw, err := base64.StdEncoding.DecodeString(m[3])
	if err != nil {
		return nil, nil, fmt.Errorf("its findings record is not base64: %w", err)
	}
	js, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), maxRecord+1))
	if err != nil {
		return nil, nil, fmt.Errorf("its findings record does not inflate: %w", err)
	}
	if len(js) > maxRecord {
		return nil, nil, errors.New("its findings record inflates past 16 MiB")
	}
	findings, assessments, err := decodeRecord(m[1], js)
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	for i, f := range findings {
		switch {
		case f.ID == "":
			return nil, nil, fmt.Errorf("finding %d in its findings record has no id", i+1)
		case f.Title == "":
			return nil, nil, fmt.Errorf("%s in its findings record has no title", f.ID)
		case seen[f.ID]:
			return nil, nil, fmt.Errorf("its findings record holds %s twice", f.ID)
		}
		seen[f.ID] = true
	}
	for i, a := range assessments {
		switch {
		case a.Status != "open" && a.Status != "addressed":
			return nil, nil, fmt.Errorf("assessment %d in its findings record has status %q, not open or addressed", i+1, a.Status)
		case a.Finding.ID == "" || a.Finding.Title == "":
			return nil, nil, fmt.Errorf("assessment %d in its findings record lacks its finding's id or title", i+1)
		}
	}
	return findings, assessments, nil
}

// decodeRecord reads version 1's bare list or version 2's object, each only in its own shape.
func decodeRecord(version string, js []byte) ([]RecordFinding, []RecordAssessment, error) {
	trimmed := bytes.TrimSpace(js)
	if version == "1" {
		var findings []RecordFinding
		if len(trimmed) == 0 || trimmed[0] != '[' {
			return nil, nil, errors.New("its findings record is not a list of findings")
		}
		if err := json.Unmarshal(js, &findings); err != nil {
			return nil, nil, fmt.Errorf("its findings record is not a list of findings: %w", err)
		}
		return findings, nil, nil
	}
	var v recordV2
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, nil, errors.New("its findings record is not findings and assessments")
	}
	if err := json.Unmarshal(js, &v); err != nil {
		return nil, nil, fmt.Errorf("its findings record is not findings and assessments: %w", err)
	}
	if v.Findings == nil || v.Assessments == nil {
		return nil, nil, errors.New("its findings record is not findings and assessments")
	}
	return v.Findings, v.Assessments, nil
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
		n, open, addressed, ok := recordCount(m[1], m[3])
		if !ok {
			continue
		}
		note := fmt.Sprintf("a copy of the %d findings above", n)
		switch n {
		case 0:
			note = "a copy of no findings"
		case 1:
			note = "a copy of the 1 finding above"
		}
		lines[i] = recordPrefix[:len(recordPrefix)-1] + ": " + note + assessedNote(open, addressed) + " -->"
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

// assessedNote names the statuses an attended round publishes under the human's name, since the confirmation shows the
// record only as this note.
func assessedNote(open, addressed int) string {
	earlier := func(n int) string {
		if n == 1 {
			return "1 earlier finding"
		}
		return fmt.Sprintf("%d earlier findings", n)
	}
	switch {
	case open == 0 && addressed == 0:
		return ""
	case addressed == 0:
		return ", plus " + earlier(open) + " still open"
	case open == 0:
		return ", plus " + earlier(addressed) + " marked addressed"
	}
	return fmt.Sprintf(", plus %s, %d still open and %d addressed", earlier(open+addressed), open, addressed)
}

func recordCount(version, data string) (findings, open, addressed int, ok bool) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return 0, 0, 0, false
	}
	js, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw)), maxRecord+1))
	if err != nil || len(js) > maxRecord {
		return 0, 0, 0, false
	}
	if version != "1" && version != "2" {
		return 0, 0, 0, false
	}
	fs, as, err := decodeRecord(version, js)
	if err != nil {
		return 0, 0, 0, false
	}
	for _, a := range as {
		if a.Status == "open" {
			open++
		} else {
			addressed++
		}
	}
	return len(fs), open, addressed, true
}

// MetaRound is the round= value on the marker line MetaSource reads, 0 when there is none.
// PublicationID is the publication id of body's current round, empty when it has none. A sticky body keeps each
// earlier round's reconciliation marker above its own, so the last one outside a fence is the current round's.
func PublicationID(body string) string {
	lines, structural := markdown.StructuralLines(body)
	for i := len(lines) - 1; i >= 0; i-- {
		if m := publicationKey.FindStringSubmatch(lines[i]); structural[i] && m != nil {
			return m[1]
		}
	}
	return ""
}

var publicationKey = regexp.MustCompile(`^<!-- loupe digest=\S+ publication=(\S+) -->$`)

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
