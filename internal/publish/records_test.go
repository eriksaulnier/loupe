package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

func sampleEnvelope() Envelope {
	return Envelope{
		Target:        EnvelopeTarget{Owner: "acme", Repo: "widgets", Number: 42, HeadSHA: "abc123", Round: 1},
		Viewer:        "reviewer",
		Action:        "comment",
		Event:         "COMMENT",
		CommitID:      "abc123",
		DraftVersion:  5,
		Digest:        "d1",
		PublicationID: "p1",
		Inline:        "all",
		Body:          "body",
		Comments:      []Comment{{Path: "a.go", Line: 3, Side: "RIGHT", StartLine: 1, StartSide: "RIGHT", Body: "c"}},
		Findings:      []EnvelopeFinding{{ID: "f-001", Title: "t", Body: "b", Location: &draft.Location{Path: "a.go", Side: "RIGHT", Line: 3}, Label: "issue", Blocking: true}},
	}
}

func TestAttemptRoundTripAndDelete(t *testing.T) {
	dir := t.TempDir()
	if _, found, err := LoadAttempt(dir); err != nil || found {
		t.Fatalf("missing attempt: found %v err %v", found, err)
	}
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	a := Attempt{Schema: RecordSchema, State: StateUnknown, StartedAt: at, UpdatedAt: at, Envelope: sampleEnvelope(),
		Confirmed: Confirmed{Version: 5, Digest: "d1", Dispositions: map[string]string{"f-001": "accepted"}}, LastError: "boom"}
	if err := SaveAttempt(dir, a); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadAttempt(dir)
	if err != nil || !found || !reflect.DeepEqual(got, a) {
		t.Fatalf("loaded %+v found %v err %v", got, found, err)
	}
	if err := DeleteAttempt(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "attempt.json")); !os.IsNotExist(err) {
		t.Fatalf("attempt.json still present: %v", err)
	}
}

func TestReceiptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, found, err := LoadReceipt(dir); err != nil || found {
		t.Fatalf("missing receipt: found %v err %v", found, err)
	}
	r := Receipt{Schema: RecordSchema, ReviewID: 7, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-7",
		Action: "comment", PostedAt: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC), Envelope: sampleEnvelope()}
	if err := SaveReceipt(dir, r); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadReceipt(dir)
	if err != nil || !found || !reflect.DeepEqual(got, r) {
		t.Fatalf("loaded %+v found %v err %v", got, found, err)
	}
}

func TestReceiptAuthorRoundTripsAndIsOmittedWhenEmpty(t *testing.T) {
	data := mustJSON(t, Receipt{Schema: RecordSchema, Envelope: sampleEnvelope()})
	if strings.Contains(data, `"author"`) {
		t.Fatalf("author must be omitted when empty: %s", data)
	}

	dir := t.TempDir()
	r := Receipt{Schema: RecordSchema, ReviewID: 7, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-7",
		Action: "comment", PostedAt: time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC), Envelope: sampleEnvelope(), Author: "github-actions[bot]"}
	if err := SaveReceipt(dir, r); err != nil {
		t.Fatal(err)
	}
	got, found, err := LoadReceipt(dir)
	if err != nil || !found || !reflect.DeepEqual(got, r) {
		t.Fatalf("loaded %+v found %v err %v", got, found, err)
	}
}

func TestRecordsRefuseWrongSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "receipt.json"), []byte(`{"schema": 2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadReceipt(dir); !isRefusal(err, refusal.Record) {
		t.Fatalf("err %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "attempt.json"), []byte(`{"schema": 1, "state": "sent"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadAttempt(dir); !isRefusal(err, refusal.Record) {
		t.Fatalf("err %v", err)
	}
}

func TestEnvelopeJSONKeys(t *testing.T) {
	data := mustJSON(t, sampleEnvelope())
	for _, want := range []string{`"target":{"owner":"acme","repo":"widgets","number":42,"headSha":"abc123","round":1}`,
		`"commitId":"abc123"`, `"draftVersion":5`, `"publicationId":"p1"`,
		`"comments":[{"path":"a.go","line":3,"side":"RIGHT","startLine":1,"startSide":"RIGHT","body":"c"}]`,
		`"findings":[{"id":"f-001","title":"t","body":"b","location":{"path":"a.go","side":"RIGHT","line":3},"label":"issue","blocking":true}]`} {
		if !regexp.MustCompile(regexp.QuoteMeta(want)).MatchString(data) {
			t.Errorf("envelope JSON lacks %s:\n%s", want, data)
		}
	}
}

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewPublicationID(t *testing.T) {
	a, b := newPublicationID(), newPublicationID()
	if !uuidV4.MatchString(a) || !uuidV4.MatchString(b) || a == b {
		t.Fatalf("ids %q %q", a, b)
	}
}

func isRefusal(err error, code refusal.Code) bool {
	r, ok := refusal.As(err)
	return ok && r.Code == code
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
