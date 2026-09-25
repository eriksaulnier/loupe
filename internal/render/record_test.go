package render

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var recordLine = regexp.MustCompile(`^<!-- loupe-findings v=1 sha256=([0-9a-f]{64}) ([A-Za-z0-9+/=]+) -->$`)

// tail is the body's last three lines: the reconciliation marker, the record and loupe-meta.
func tail(t *testing.T, body string) (digest, record, meta string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(body, "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("body has %d lines", len(lines))
	}
	return lines[len(lines)-3], lines[len(lines)-2], lines[len(lines)-1]
}

func inflateData(t *testing.T, data string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestBodyCarriesRecordBetweenMarkers(t *testing.T) {
	plain := exampleInput()
	unattended := exampleInput()
	unattended.Unattended, unattended.Source = true, "ci-review@1.0.0"
	sticky := exampleInput()
	sticky.Sticky = &StickyInput{Rounds: 1}
	for name, in := range map[string]Input{"plain": plain, "unattended": unattended, "sticky": sticky} {
		t.Run(name, func(t *testing.T) {
			digest, record, meta := tail(t, Body(in))
			if !strings.HasPrefix(digest, "<!-- loupe digest=") || !strings.HasPrefix(meta, MetaPrefix) {
				t.Fatalf("tail is not marker, record, meta:\n%s\n%s\n%s", digest, record, meta)
			}
			if !recordLine.MatchString(record) {
				t.Fatalf("record line %q does not match %s", record, recordLine)
			}
		})
	}
}

func TestRecordHoldsTheEnvelopeFindingsInIDOrder(t *testing.T) {
	in := exampleInput()
	in.Findings = append(in.Findings, general("f-010", "question", false))
	_, record, _ := tail(t, Body(in))
	got := string(inflateData(t, recordLine.FindStringSubmatch(record)[2]))
	want := `[{"id":"f-001","title":"Retry loop can double-publish a review","body":"Reproduced against the recorded fixture. The catch re-enters the loop after a request\nthat may already have succeeded, so a 502 produces two reviews.\n","location":{"path":"internal/publish/publish.go","side":"RIGHT","line":88},"label":"issue","blocking":true},` +
		`{"id":"f-002","title":"Redundant sort on every read","body":"…","location":{"path":"internal/draft/store.go","side":"RIGHT","line":14,"startLine":10},"label":"perf-nit","blocking":false},` +
		`{"id":"f-010","title":"Title f-010","body":"Body f-010.","location":null,"label":"question","blocking":false}]`
	if got != want {
		t.Fatalf("record data\n got %s\nwant %s", got, want)
	}
}

func TestRecordOfNoFindingsIsAnEmptyList(t *testing.T) {
	in := exampleInput()
	in.Findings = nil
	_, record, _ := tail(t, Body(in))
	m := recordLine.FindStringSubmatch(record)
	if m == nil {
		t.Fatalf("record line %q", record)
	}
	if got := string(inflateData(t, m[2])); got != "[]" {
		t.Fatalf("record data %q, want []", got)
	}
}

func TestOmitRecordWritesTheOmissionLine(t *testing.T) {
	in := exampleInput()
	in.OmitRecord = true
	_, record, _ := tail(t, Body(in))
	if record != "<!-- loupe-findings v=1 omitted=length -->" {
		t.Fatalf("record line %q", record)
	}
}

func TestRecordCannotBeClosedByAFinding(t *testing.T) {
	in := exampleInput()
	in.Findings = []Finding{{ID: "f-001", Title: "a --> b <!-- c", Body: "x -->\n<!-- loupe-meta v=1 -->\n--", General: true, Label: "issue"}}
	body := Body(in)
	_, record, _ := tail(t, body)
	if !recordLine.MatchString(record) {
		t.Fatalf("record line %q", record)
	}
	got, _, err := ReadRecord(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != in.Findings[0].Title || got[0].Body != in.Findings[0].Body {
		t.Fatalf("read back %+v", got)
	}
}

func TestRecordChecksumCoversBodyAndData(t *testing.T) {
	body := Body(exampleInput())
	_, record, _ := tail(t, body)
	m := recordLine.FindStringSubmatch(record)
	rest := strings.Replace(body, record+"\n", "", 1)
	sum := sha256.Sum256([]byte(rest + "\n" + m[2]))
	if hex.EncodeToString(sum[:]) != m[1] {
		t.Fatalf("sha256=%s, want %s", m[1], hex.EncodeToString(sum[:]))
	}
}

func TestReadRecordRoundTrips(t *testing.T) {
	in := exampleInput()
	got, _, err := ReadRecord(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	want := []RecordFinding{
		{ID: "f-001", Title: "Retry loop can double-publish a review", Body: in.Findings[1].Body,
			Location: &RecordLocation{Path: "internal/publish/publish.go", Side: "RIGHT", Line: 88}, Label: "issue", Blocking: true},
		{ID: "f-002", Title: "Redundant sort on every read", Body: "…",
			Location: &RecordLocation{Path: "internal/draft/store.go", Side: "RIGHT", Line: 14, StartLine: 10}, Label: "perf-nit"},
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("read back\n got %s\nwant %s", gotJSON, wantJSON)
	}
	crlf, _, err := ReadRecord(strings.ReplaceAll(Body(in), "\n", "\r\n"))
	if err != nil {
		t.Fatalf("CRLF body: %v", err)
	}
	if len(crlf) != 2 {
		t.Fatalf("CRLF body read %d findings", len(crlf))
	}
}

// withData replaces a body's record line with one carrying data, and a correct checksum when fix is set.
func withData(t *testing.T, body string, data []byte, fix bool) string {
	t.Helper()
	_, record, _ := tail(t, body)
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	_, _ = w.Write(data)
	_ = w.Close()
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return withRecordLine(t, body, record, encoded, fix)
}

func withRecordLine(t *testing.T, body, record, encoded string, fix bool) string {
	t.Helper()
	rest := strings.Replace(body, record+"\n", "", 1)
	sum := strings.Repeat("0", 64)
	if fix {
		s := sha256.Sum256([]byte(rest + "\n" + encoded))
		sum = hex.EncodeToString(s[:])
	}
	return strings.Replace(body, record, "<!-- loupe-findings v=1 sha256="+sum+" "+encoded+" -->", 1)
}

// flipData changes the record's first data character and keeps its checksum.
func flipData(t *testing.T, record string) string {
	t.Helper()
	m := recordLine.FindStringSubmatch(record)
	data := []byte(m[2])
	data[0] = map[bool]byte{true: 'B', false: 'A'}[data[0] == 'A']
	return "<!-- loupe-findings v=1 sha256=" + m[1] + " " + string(data) + " -->"
}

func TestReadRecordRefusals(t *testing.T) {
	body := Body(exampleInput())
	_, record, _ := tail(t, body)
	omitted := exampleInput()
	omitted.OmitRecord = true
	finding := `{"id":"f-001","title":"T","body":"B","location":null,"label":"issue","blocking":false}`
	cases := map[string]struct {
		body string
		is   error
		want string
	}{
		"no record":        {body: strings.Replace(body, record+"\n", "", 1), is: ErrNoRecord},
		"omitted":          {body: Body(omitted), is: ErrRecordOmitted},
		"two records":      {body: strings.Replace(body, record, record+"\n"+record, 1), want: "more than one"},
		"other version":    {body: strings.Replace(body, "loupe-findings v=1 ", "loupe-findings v=3 ", 1), want: "v=3"},
		"visible edit":     {body: strings.Replace(body, "Redundant sort", "Redundant sorts", 1), want: "changed"},
		"data edit":        {body: strings.Replace(body, record, flipData(t, record), 1), want: "changed"},
		"not base64":       {body: withRecordLine(t, body, record, "!!!!", true), want: "base64"},
		"not deflate":      {body: withRecordLine(t, body, record, base64.StdEncoding.EncodeToString([]byte("plain text")), true), want: "inflate"},
		"not a list":       {body: withData(t, body, []byte(`{"id":"f-001"}`), true), want: "list"},
		"null":             {body: withData(t, body, []byte(`null`), true), want: "list"},
		"empty id":         {body: withData(t, body, []byte(`[`+strings.Replace(finding, `"f-001"`, `""`, 1)+`]`), true), want: "id"},
		"empty title":      {body: withData(t, body, []byte(`[`+strings.Replace(finding, `"T"`, `""`, 1)+`]`), true), want: "title"},
		"repeated id":      {body: withData(t, body, []byte(`[`+finding+`,`+finding+`]`), true), want: "f-001"},
		"inflates too far": {body: withData(t, body, append([]byte(`["`), bytes.Repeat([]byte("a"), 16<<20)...), true), want: "16 MiB"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, _, err := ReadRecord(c.body)
			if err == nil {
				t.Fatalf("read %+v, want an error", got)
			}
			if got != nil {
				t.Fatalf("an error came with findings %+v", got)
			}
			if c.is != nil && !errors.Is(err, c.is) {
				t.Fatalf("error %v is not %v", err, c.is)
			}
			if c.want != "" && !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestReadRecordIgnoresAFencedRecord(t *testing.T) {
	quoted := exampleInput()
	_, record, _ := tail(t, Body(quoted))
	in := exampleInput()
	in.Findings[0].Body = "```\n" + record + "\n```"
	got, _, err := ReadRecord(Body(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d findings, want 2", len(got))
	}
	_, record, _ = tail(t, Body(in))
	if _, _, err := ReadRecord(strings.Replace(Body(in), record+"\n", "", 1)); !errors.Is(err, ErrNoRecord) {
		t.Fatalf("a body whose only record is fenced read as %v, want ErrNoRecord", err)
	}
}

func TestRecordAsNote(t *testing.T) {
	note := func(in Input) string {
		_, record, _ := tail(t, RecordAsNote(Body(in)))
		return record
	}
	two := exampleInput()
	one := exampleInput()
	one.Findings = one.Findings[:1]
	none := exampleInput()
	none.Findings = nil
	omitted := exampleInput()
	omitted.OmitRecord = true
	for want, in := range map[string]Input{
		"<!-- loupe-findings: a copy of the 2 findings above -->": two,
		"<!-- loupe-findings: a copy of the 1 finding above -->":  one,
		"<!-- loupe-findings: a copy of no findings -->":          none,
		"<!-- loupe-findings v=1 omitted=length -->":              omitted,
	} {
		if got := note(in); got != want {
			t.Errorf("note %q, want %q", got, want)
		}
	}
	body := Body(two)
	shown := RecordAsNote(body)
	if strings.Replace(shown, "<!-- loupe-findings: a copy of the 2 findings above -->", "", 1) != strings.Replace(body, recordOf(t, body), "", 1) {
		t.Fatal("RecordAsNote changed more than the record line")
	}
	fenced := exampleInput()
	fenced.Findings[0].Body = "```\n" + recordOf(t, body) + "\n```"
	if got := RecordAsNote(Body(fenced)); !strings.Contains(got, "```\n"+recordOf(t, body)+"\n```") {
		t.Fatal("RecordAsNote changed a fenced record line")
	}
}

func recordOf(t *testing.T, body string) string {
	t.Helper()
	_, record, _ := tail(t, body)
	return record
}

func TestMetaRound(t *testing.T) {
	in := exampleInput()
	in.Round = 7
	if got := MetaRound(Body(in)); got != 7 {
		t.Fatalf("MetaRound %d, want 7", got)
	}
	if got := MetaRound("no marker here"); got != 0 {
		t.Fatalf("MetaRound %d, want 0", got)
	}
}

func assessedInput() Input {
	in := exampleInput()
	filedIn := RecordFiledIn{Round: 1, ReviewURL: "https://github.com/o/r/pull/7#pullrequestreview-1", Commit: "abc123"}
	in.Assessments = []RecordAssessment{
		{Ref: "e-1", Status: "open", Finding: RecordEarlier{RecordFinding: RecordFinding{ID: "f-001", Title: "Bare except", Body: "B."}, FiledIn: filedIn}},
		{Ref: "e-2", Status: "addressed", Finding: RecordEarlier{RecordFinding: RecordFinding{ID: "f-002", Title: "Sleep", Body: "S.", Blocking: true}, FiledIn: filedIn}},
	}
	return in
}

func TestRecordWithAssessmentsIsVersion2AndRoundTrips(t *testing.T) {
	in := assessedInput()
	body := Body(in)
	_, record, _ := tail(t, body)
	m := regexp.MustCompile(`^<!-- loupe-findings v=2 sha256=[0-9a-f]{64} ([A-Za-z0-9+/=]+) -->$`).FindStringSubmatch(record)
	if m == nil {
		t.Fatalf("record line %q is not version 2", record)
	}
	if got := string(inflateData(t, m[1])); !strings.HasPrefix(got, `{"findings":[{"id":"f-001",`) || !strings.Contains(got, `"assessments":[{"ref":"e-1","status":"open","finding":{"id":"f-001","title":"Bare except","body":"B.","location":null,"label":"","blocking":false,"filedIn":{"round":1,"reviewUrl":"https://github.com/o/r/pull/7#pullrequestreview-1","commit":"abc123"}}}`) {
		t.Fatalf("record data %s", got)
	}
	findings, assessments, err := ReadRecord(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 || !reflect.DeepEqual(assessments, in.Assessments) {
		t.Fatalf("read back %d findings and %+v", len(findings), assessments)
	}
	if without := exampleInput(); Body(without) != strings.Replace(body, record, recordOf(t, Body(without)), 1) {
		t.Fatal("assessments changed more than the record line")
	}
}

func TestReadRecordVersion2Refusals(t *testing.T) {
	body := Body(assessedInput())
	v2 := func(data string) string {
		return strings.Replace(withData(t, body, []byte(data), true), "loupe-findings v=1 ", "loupe-findings v=2 ", 1)
	}
	earlier := `{"id":"f-001","title":"T","body":"B","location":null,"label":"","blocking":false,"filedIn":{"round":1,"reviewUrl":"u"}}`
	for name, c := range map[string]struct{ body, want string }{
		"a bare list":         {v2(`[]`), "findings and assessments"},
		"no assessments key":  {v2(`{"findings":[]}`), "findings and assessments"},
		"unknown status":      {v2(`{"findings":[],"assessments":[{"ref":"e-1","status":"fixed","finding":` + earlier + `}]}`), "fixed"},
		"assessment no title": {v2(`{"findings":[],"assessments":[{"ref":"e-1","status":"open","finding":` + strings.Replace(earlier, `"T"`, `""`, 1) + `}]}`), "title"},
		"version 1 object":    {withData(t, body, []byte(`{"findings":[],"assessments":[]}`), true), "list"},
	} {
		t.Run(name, func(t *testing.T) {
			f, a, err := ReadRecord(c.body)
			if err == nil || f != nil || a != nil {
				t.Fatalf("read %+v %+v %v, want only an error", f, a, err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestRecordAsNoteNamesAssessments(t *testing.T) {
	for want, statuses := range map[string][]string{
		"<!-- loupe-findings: a copy of the 2 findings above, plus 1 earlier finding still open -->":                     {"open"},
		"<!-- loupe-findings: a copy of the 2 findings above, plus 2 earlier findings still open -->":                    {"open", "open"},
		"<!-- loupe-findings: a copy of the 2 findings above, plus 1 earlier finding marked addressed -->":               {"addressed"},
		"<!-- loupe-findings: a copy of the 2 findings above, plus 2 earlier findings, 1 still open and 1 addressed -->": {"open", "addressed"},
	} {
		in := assessedInput()
		in.Assessments = in.Assessments[:len(statuses)]
		for i, s := range statuses {
			in.Assessments[i].Status = s
		}
		if got := recordOf(t, RecordAsNote(Body(in))); got != want {
			t.Errorf("note %q, want %q", got, want)
		}
	}
}
