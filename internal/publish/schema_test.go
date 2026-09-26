package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// versionedFiles is every run file that carries a schema, with the schema this loupe writes and its reader.
var versionedFiles = []struct {
	file   string
	schema int
	load   func(dir string) error
}{
	{"draft.json", draft.SchemaVersion, func(dir string) error { _, err := draft.Load(dir); return err }},
	{"handback.json", draft.HandBackSchema, func(dir string) error { _, err := draft.LoadHandBack(dir); return err }},
	{"target.json", run.TargetSchema, func(dir string) error { _, err := run.LoadTarget(dir); return err }},
	{attemptFile, RecordSchema, func(dir string) error { _, _, err := LoadAttempt(dir); return err }},
	{receiptFile, RecordSchema, func(dir string) error { _, _, err := LoadReceipt(dir); return err }},
	{run.PreviousFile, PreviousSchema, func(dir string) error { _, _, err := LoadPrevious(dir); return err }},
	{run.CommentsFile, CommentsSchema, func(dir string) error { _, _, err := LoadComments(dir); return err }},
}

func TestReadersRefuseANewerSchemaWithAnUpgrade(t *testing.T) {
	for _, f := range versionedFiles {
		t.Run(f.file, func(t *testing.T) {
			newer := f.schema + 1
			dir := t.TempDir()
			path := filepath.Join(dir, f.file)
			if err := os.WriteFile(path, fmt.Appendf(nil, `{"schema": %d, "future": true}`, newer), 0o600); err != nil {
				t.Fatal(err)
			}
			r, ok := refusal.As(f.load(dir))
			want := fmt.Sprintf("schema is %d, this loupe reads up to %d", newer, f.schema)
			if !ok || r.Code != refusal.Record || !strings.Contains(r.Message, want) || r.Fix != "upgrade loupe" {
				t.Fatalf("got %+v, want a record refusal saying %q with fix %q", r, want, "upgrade loupe")
			}
		})
	}
}

func TestReadersRefuseAnUnknownFieldAtTheirOwnSchema(t *testing.T) {
	for _, f := range versionedFiles {
		t.Run(f.file, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, f.file)
			if err := os.WriteFile(path, fmt.Appendf(nil, `{"schema": %d, "future": true}`, f.schema), 0o600); err != nil {
				t.Fatal(err)
			}
			r, ok := refusal.As(f.load(dir))
			if !ok || r.Code != refusal.Record || !strings.Contains(r.Message, `unknown field "future"`) || r.Fix != "inspect it with: cat "+path {
				t.Fatalf("got %+v", r)
			}
		})
	}
}

// fieldSets holds each versioned struct's JSON field paths for every schema it has written. A field added, removed or
// renamed without a bump fails here, because an older loupe would refuse or silently drop it. A bump adds the new
// schema's set and keeps the older ones.
var fieldSets = []struct {
	typ    reflect.Type
	schema int
	fields map[int][]string
}{
	{reflect.TypeFor[draft.Draft](), draft.SchemaVersion, map[int][]string{
		1: {
			"decisions", "decisions.at", "decisions.decision", "decisions.findingId", "decisions.findingRev",
			"findings", "findings.blocking", "findings.body", "findings.by", "findings.confidence",
			"findings.createdAt", "findings.general", "findings.history", "findings.history.at",
			"findings.history.by", "findings.history.changed", "findings.id", "findings.impact", "findings.included",
			"findings.label", "findings.location", "findings.location.line", "findings.location.path",
			"findings.location.side", "findings.location.startLine", "findings.references", "findings.rev",
			"findings.severity", "findings.suggestedFix", "findings.title", "findings.updatedAt",
			"findings.verified", "notes", "notes.at", "notes.body", "notes.closedAt", "notes.findingId", "notes.id",
			"notes.status", "replies", "replies.at", "replies.body", "replies.by", "replies.id", "replies.noteId",
			"schema", "summary", "version",
		},
	}},
	{reflect.TypeFor[draft.HandBack](), draft.HandBackSchema, map[int][]string{
		1: {
			"notes", "schema",
		},
	}},
	{reflect.TypeFor[run.Target](), run.TargetSchema, map[int][]string{
		1: {
			"author", "baseRef", "baseSha", "capturedAt", "clonePath", "diffSha256", "headRef", "headSha",
			"mergeBaseSha", "model", "number", "owner", "previousRound", "repo", "round", "schema", "source",
			"title", "url", "viewer",
		},
	}},
	{reflect.TypeFor[Attempt](), RecordSchema, map[int][]string{
		1: {
			"confirmed", "confirmed.digest", "confirmed.dispositions", "confirmed.version", "envelope",
			"envelope.action", "envelope.body", "envelope.comments", "envelope.comments.body",
			"envelope.comments.line", "envelope.comments.path", "envelope.comments.side",
			"envelope.comments.startLine", "envelope.comments.startSide", "envelope.commitId", "envelope.digest",
			"envelope.draftVersion", "envelope.editReviewId", "envelope.event", "envelope.findings",
			"envelope.findings.blocking", "envelope.findings.body", "envelope.findings.id",
			"envelope.findings.label", "envelope.findings.location", "envelope.findings.location.line",
			"envelope.findings.location.path", "envelope.findings.location.side",
			"envelope.findings.location.startLine", "envelope.findings.title", "envelope.inline",
			"envelope.publicationId", "envelope.target", "envelope.target.headSha", "envelope.target.number",
			"envelope.target.owner", "envelope.target.repo", "envelope.target.round", "envelope.viewer", "lastError",
			"schema", "startedAt", "state", "updatedAt",
		},
	}},
	{reflect.TypeFor[Receipt](), RecordSchema, map[int][]string{
		1: {
			"action", "author", "edited", "envelope", "envelope.action", "envelope.body", "envelope.comments",
			"envelope.comments.body", "envelope.comments.line", "envelope.comments.path", "envelope.comments.side",
			"envelope.comments.startLine", "envelope.comments.startSide", "envelope.commitId", "envelope.digest",
			"envelope.draftVersion", "envelope.editReviewId", "envelope.event", "envelope.findings",
			"envelope.findings.blocking", "envelope.findings.body", "envelope.findings.id",
			"envelope.findings.label", "envelope.findings.location", "envelope.findings.location.line",
			"envelope.findings.location.path", "envelope.findings.location.side",
			"envelope.findings.location.startLine", "envelope.findings.title", "envelope.inline",
			"envelope.publicationId", "envelope.target", "envelope.target.headSha", "envelope.target.number",
			"envelope.target.owner", "envelope.target.repo", "envelope.target.round", "envelope.viewer", "postedAt",
			"reviewId", "reviewUrl", "schema",
		},
	}},
	{reflect.TypeFor[Previous](), PreviousSchema, map[int][]string{
		1: {
			"findings", "findings.blocking", "findings.body", "findings.id", "findings.label", "findings.location",
			"findings.location.line", "findings.location.path", "findings.location.side",
			"findings.location.startLine", "findings.title", "found", "reason", "reviewId", "reviewUrl", "round",
			"schema",
		},
	}},
	{reflect.TypeFor[Comments](), CommentsSchema, map[int][]string{
		1: {
			"comments", "comments.author", "comments.body", "comments.createdAt", "comments.url", "excludedReviews",
			"read", "reason", "reviews", "reviews.author", "reviews.body", "reviews.id", "reviews.state",
			"reviews.submittedAt", "reviews.url", "schema", "threads", "threads.comments", "threads.comments.author",
			"threads.comments.body", "threads.comments.createdAt", "threads.comments.url", "threads.line",
			"threads.originalLine", "threads.outdated", "threads.path", "threads.resolved", "threads.side",
		},
	}},
}

func TestEachRecordsFieldSetIsPinnedToItsSchema(t *testing.T) {
	for _, s := range fieldSets {
		t.Run(s.typ.String(), func(t *testing.T) {
			want, ok := s.fields[s.schema]
			if !ok {
				t.Fatalf("no field set is pinned for schema %d", s.schema)
			}
			var got []string
			jsonFields(s.typ, "", &got)
			slices.Sort(got)
			added := slices.DeleteFunc(slices.Clone(got), func(f string) bool { return slices.Contains(want, f) })
			removed := slices.DeleteFunc(slices.Clone(want), func(f string) bool { return slices.Contains(got, f) })
			if len(added) > 0 || len(removed) > 0 {
				t.Fatalf("schema %d fields changed without a bump: added %q, removed %q", s.schema, added, removed)
			}
		})
	}
}

// jsonFields walks into slices, maps and pointers, since a field added to a nested struct changes the file as much as
// one added at the top.
func jsonFields(t reflect.Type, prefix string, out *[]string) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		*out = append(*out, prefix+name)
		ft := f.Type
		for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft != reflect.TypeFor[time.Time]() {
			jsonFields(ft, prefix+name+".", out)
		}
	}
}

// A loupe that reads an older file writes its own schema on save, so an older number never labels the newer fields.
func TestWritersStampTheirOwnSchema(t *testing.T) {
	dir := t.TempDir()
	if err := SaveAttempt(dir, Attempt{State: StateInFlight, StartedAt: fixtureNow, UpdatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	defer func(s int) { recordSchema = s }(recordSchema)
	recordSchema = 2
	a, _, err := LoadAttempt(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.State = StateUnknown
	if err := SaveAttempt(dir, a); err != nil {
		t.Fatal(err)
	}
	if err := SaveReceipt(dir, Receipt{Schema: 1}); err != nil {
		t.Fatal(err)
	}
	previous, err := EncodePrevious(Previous{Reason: "why"})
	if err != nil {
		t.Fatal(err)
	}
	comments, err := EncodeComments(Comments{Reason: "why"})
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), "run")
	target := fixtureTarget()
	target.Schema = 0
	if err := run.CreateRun(runDir, target, nil, []byte("{}\n"), nil); err != nil {
		t.Fatal(err)
	}
	targetJSON, err := os.ReadFile(filepath.Join(runDir, "target.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		file string
		data []byte
		want int
	}{
		{attemptFile, readFile(t, filepath.Join(dir, attemptFile)), 2},
		{receiptFile, readFile(t, filepath.Join(dir, receiptFile)), 2},
		{run.PreviousFile, previous, PreviousSchema},
		{run.CommentsFile, comments, CommentsSchema},
		{"target.json", targetJSON, run.TargetSchema},
	} {
		var head struct {
			Schema int `json:"schema"`
		}
		if err := json.Unmarshal(c.data, &head); err != nil || head.Schema != c.want {
			t.Errorf("%s saved with schema %d (%v), want %d", c.file, head.Schema, err, c.want)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
