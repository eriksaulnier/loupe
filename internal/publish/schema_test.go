package publish

import (
	"context"
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
	"github.com/eriksaulnier/loupe/internal/github"
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

// fieldSets holds each versioned struct's JSON field paths, with their whole tags, for every schema it has written. A
// field added, removed, renamed or retagged without a bump fails here, because an older loupe would refuse or silently
// drop it. A bump adds the new schema's set and keeps the older ones.
var fieldSets = []struct {
	typ    reflect.Type
	schema int
	fields map[int][]string
}{
	{reflect.TypeFor[draft.Draft](), draft.SchemaVersion, map[int][]string{
		1: {
			"decisions", "decisions.at", "decisions.decision", "decisions.findingId", "decisions.findingRev",
			"findings", "findings.blocking", "findings.body", "findings.by", "findings.confidence,omitempty",
			"findings.createdAt", "findings.general", "findings.history", "findings.history.at",
			"findings.history.by", "findings.history.changed", "findings.id", "findings.impact,omitempty",
			"findings.included", "findings.label,omitempty", "findings.location,omitempty", "findings.location.line",
			"findings.location.path", "findings.location.side", "findings.location.startLine,omitempty",
			"findings.references,omitempty", "findings.rev", "findings.severity,omitempty",
			"findings.suggestedFix,omitempty", "findings.title", "findings.updatedAt", "findings.verified,omitempty",
			"notes", "notes.at", "notes.body", "notes.closedAt,omitempty", "notes.findingId", "notes.id",
			"notes.status", "replies", "replies.at", "replies.body", "replies.by", "replies.id", "replies.noteId",
			"schema", "summary", "version",
		},
		2: {
			"assessedAgainst,omitempty", "assessedAgainst.from", "assessedAgainst.publicationId,omitempty",
			"assessedAgainst.reviewUrl", "assessedAgainst.round", "assessments,omitempty", "assessments.finding",
			"assessments.finding.blocking", "assessments.finding.body", "assessments.finding.filedIn",
			"assessments.finding.filedIn.commit,omitempty", "assessments.finding.filedIn.reviewUrl",
			"assessments.finding.filedIn.round", "assessments.finding.id", "assessments.finding.label",
			"assessments.finding.location", "assessments.finding.location.line", "assessments.finding.location.path",
			"assessments.finding.location.side", "assessments.finding.location.startLine,omitempty",
			"assessments.finding.title", "assessments.ref", "assessments.status", "decisions", "decisions.at",
			"decisions.decision", "decisions.findingId", "decisions.findingRev", "findings", "findings.blocking",
			"findings.body", "findings.by", "findings.confidence,omitempty", "findings.createdAt", "findings.general",
			"findings.history", "findings.history.at", "findings.history.by", "findings.history.changed", "findings.id",
			"findings.impact,omitempty", "findings.included", "findings.label,omitempty", "findings.location,omitempty",
			"findings.location.line", "findings.location.path", "findings.location.side",
			"findings.location.startLine,omitempty", "findings.references,omitempty", "findings.rev",
			"findings.severity,omitempty", "findings.suggestedFix,omitempty", "findings.title", "findings.updatedAt",
			"findings.verified,omitempty", "notes", "notes.at", "notes.body", "notes.closedAt,omitempty",
			"notes.findingId", "notes.id", "notes.status", "replies", "replies.at", "replies.body", "replies.by",
			"replies.id", "replies.noteId", "schema", "summary", "version",
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
			"mergeBaseSha", "model,omitempty", "number", "owner", "previousRound,omitempty", "repo", "round",
			"schema", "source,omitempty", "title", "url", "viewer",
		},
	}},
	{reflect.TypeFor[Attempt](), RecordSchema, map[int][]string{
		1: {
			"confirmed", "confirmed.digest", "confirmed.dispositions", "confirmed.version", "envelope",
			"envelope.action", "envelope.body", "envelope.comments", "envelope.comments.body",
			"envelope.comments.line", "envelope.comments.path", "envelope.comments.side",
			"envelope.comments.startLine,omitempty", "envelope.comments.startSide,omitempty", "envelope.commitId",
			"envelope.digest", "envelope.draftVersion", "envelope.editReviewId,omitempty", "envelope.event",
			"envelope.findings", "envelope.findings.blocking", "envelope.findings.body", "envelope.findings.id",
			"envelope.findings.label", "envelope.findings.location", "envelope.findings.location.line",
			"envelope.findings.location.path", "envelope.findings.location.side",
			"envelope.findings.location.startLine,omitempty", "envelope.findings.title", "envelope.inline",
			"envelope.publicationId", "envelope.target", "envelope.target.headSha", "envelope.target.number",
			"envelope.target.owner", "envelope.target.repo", "envelope.target.round", "envelope.viewer",
			"lastError,omitempty", "schema", "startedAt", "state", "updatedAt",
		},
		2: {
			"confirmed", "confirmed.digest", "confirmed.dispositions", "confirmed.version", "envelope",
			"envelope.action", "envelope.assessments,omitempty", "envelope.assessments.finding",
			"envelope.assessments.finding.blocking", "envelope.assessments.finding.body",
			"envelope.assessments.finding.filedIn", "envelope.assessments.finding.filedIn.commit,omitempty",
			"envelope.assessments.finding.filedIn.reviewUrl", "envelope.assessments.finding.filedIn.round",
			"envelope.assessments.finding.id", "envelope.assessments.finding.label",
			"envelope.assessments.finding.location", "envelope.assessments.finding.location.line",
			"envelope.assessments.finding.location.path", "envelope.assessments.finding.location.side",
			"envelope.assessments.finding.location.startLine,omitempty", "envelope.assessments.finding.title",
			"envelope.assessments.ref", "envelope.assessments.status", "envelope.body", "envelope.comments",
			"envelope.comments.body", "envelope.comments.line", "envelope.comments.path", "envelope.comments.side",
			"envelope.comments.startLine,omitempty", "envelope.comments.startSide,omitempty", "envelope.commitId",
			"envelope.digest", "envelope.draftVersion", "envelope.editReviewId,omitempty", "envelope.event",
			"envelope.findings", "envelope.findings.blocking", "envelope.findings.body", "envelope.findings.id",
			"envelope.findings.label", "envelope.findings.location", "envelope.findings.location.line",
			"envelope.findings.location.path", "envelope.findings.location.side",
			"envelope.findings.location.startLine,omitempty", "envelope.findings.title", "envelope.inline",
			"envelope.publicationId", "envelope.target", "envelope.target.headSha", "envelope.target.number",
			"envelope.target.owner", "envelope.target.repo", "envelope.target.round", "envelope.viewer",
			"lastError,omitempty", "schema", "startedAt", "state", "updatedAt",
		},
	}},
	{reflect.TypeFor[Receipt](), RecordSchema, map[int][]string{
		1: {
			"action", "author,omitempty", "edited,omitempty", "envelope", "envelope.action", "envelope.body",
			"envelope.comments", "envelope.comments.body", "envelope.comments.line", "envelope.comments.path",
			"envelope.comments.side", "envelope.comments.startLine,omitempty",
			"envelope.comments.startSide,omitempty", "envelope.commitId", "envelope.digest", "envelope.draftVersion",
			"envelope.editReviewId,omitempty", "envelope.event", "envelope.findings", "envelope.findings.blocking",
			"envelope.findings.body", "envelope.findings.id", "envelope.findings.label",
			"envelope.findings.location", "envelope.findings.location.line", "envelope.findings.location.path",
			"envelope.findings.location.side", "envelope.findings.location.startLine,omitempty",
			"envelope.findings.title", "envelope.inline", "envelope.publicationId", "envelope.target",
			"envelope.target.headSha", "envelope.target.number", "envelope.target.owner", "envelope.target.repo",
			"envelope.target.round", "envelope.viewer", "postedAt", "reviewId", "reviewUrl", "schema",
		},
		2: {
			"action", "author,omitempty", "edited,omitempty", "envelope", "envelope.action",
			"envelope.assessments,omitempty", "envelope.assessments.finding", "envelope.assessments.finding.blocking",
			"envelope.assessments.finding.body", "envelope.assessments.finding.filedIn",
			"envelope.assessments.finding.filedIn.commit,omitempty", "envelope.assessments.finding.filedIn.reviewUrl",
			"envelope.assessments.finding.filedIn.round", "envelope.assessments.finding.id",
			"envelope.assessments.finding.label", "envelope.assessments.finding.location",
			"envelope.assessments.finding.location.line", "envelope.assessments.finding.location.path",
			"envelope.assessments.finding.location.side", "envelope.assessments.finding.location.startLine,omitempty",
			"envelope.assessments.finding.title", "envelope.assessments.ref", "envelope.assessments.status",
			"envelope.body", "envelope.comments", "envelope.comments.body", "envelope.comments.line",
			"envelope.comments.path", "envelope.comments.side", "envelope.comments.startLine,omitempty",
			"envelope.comments.startSide,omitempty", "envelope.commitId", "envelope.digest", "envelope.draftVersion",
			"envelope.editReviewId,omitempty", "envelope.event", "envelope.findings", "envelope.findings.blocking",
			"envelope.findings.body", "envelope.findings.id", "envelope.findings.label", "envelope.findings.location",
			"envelope.findings.location.line", "envelope.findings.location.path", "envelope.findings.location.side",
			"envelope.findings.location.startLine,omitempty", "envelope.findings.title", "envelope.inline",
			"envelope.publicationId", "envelope.target", "envelope.target.headSha", "envelope.target.number",
			"envelope.target.owner", "envelope.target.repo", "envelope.target.round", "envelope.viewer", "postedAt",
			"reviewId", "reviewUrl", "schema",
		},
	}},
	{reflect.TypeFor[Previous](), PreviousSchema, map[int][]string{
		1: {
			"findings", "findings.blocking", "findings.body", "findings.id", "findings.label", "findings.location",
			"findings.location.line", "findings.location.path", "findings.location.side",
			"findings.location.startLine,omitempty", "findings.title", "found", "reason,omitempty",
			"reviewId,omitempty", "reviewUrl,omitempty", "round,omitempty", "schema",
		},
		2: {
			"assessments,omitempty", "assessments.finding", "assessments.finding.blocking", "assessments.finding.body",
			"assessments.finding.filedIn", "assessments.finding.filedIn.commit,omitempty",
			"assessments.finding.filedIn.reviewUrl", "assessments.finding.filedIn.round", "assessments.finding.id",
			"assessments.finding.label", "assessments.finding.location", "assessments.finding.location.line",
			"assessments.finding.location.path", "assessments.finding.location.side",
			"assessments.finding.location.startLine,omitempty", "assessments.finding.title", "assessments.ref",
			"assessments.status", "commit,omitempty", "findings", "findings.blocking", "findings.body", "findings.id",
			"findings.label", "findings.location", "findings.location.line", "findings.location.path",
			"findings.location.side", "findings.location.startLine,omitempty", "findings.title", "found",
			"publicationId,omitempty", "reason,omitempty", "reviewId,omitempty", "reviewUrl,omitempty",
			"round,omitempty", "schema",
		},
	}},
	{reflect.TypeFor[Comments](), CommentsSchema, map[int][]string{
		1: {
			"comments,omitempty", "comments.author", "comments.body", "comments.createdAt,omitzero", "comments.url",
			"excludedReviews,omitempty", "read", "reason,omitempty", "reviews,omitempty", "reviews.author",
			"reviews.body", "reviews.id", "reviews.state", "reviews.submittedAt,omitzero", "reviews.url", "schema",
			"threads,omitempty", "threads.comments", "threads.comments.author", "threads.comments.body",
			"threads.comments.createdAt,omitzero", "threads.comments.url", "threads.line,omitempty",
			"threads.originalLine,omitempty", "threads.outdated", "threads.path", "threads.resolved",
			"threads.side,omitempty",
		},
	}},
	// tagProbe is not a record. It pins that a tag's options count, so an omitempty change without a bump fails.
	{reflect.TypeFor[tagProbe](), 1, map[int][]string{1: {"name,omitempty"}}},
}

type tagProbe struct {
	Name string `json:"name,omitempty"`
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
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if !f.IsExported() || name == "-" {
			continue
		}
		if name == "" {
			name, tag = f.Name, f.Name+tag
		}
		*out = append(*out, prefix+tag)
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
	if err := SaveAttempt(dir, &Attempt{State: StateInFlight, StartedAt: fixtureNow, UpdatedAt: fixtureNow}); err != nil {
		t.Fatal(err)
	}
	defer func(s int) { recordSchema = s }(recordSchema)
	next := RecordSchema + 1
	recordSchema = next
	a, _, err := LoadAttempt(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.State = StateUnknown
	if err := SaveAttempt(dir, &a); err != nil {
		t.Fatal(err)
	}
	r := Receipt{Schema: 1}
	if err := SaveReceipt(dir, &r); err != nil {
		t.Fatal(err)
	}
	gh, client := newFake(t)
	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED",
		Body: markedBody(testDigest, testPublication)})
	reconciled, err := Reconcile(context.Background(), client, unknownAttempt())
	if err != nil || reconciled == nil {
		t.Fatalf("reconciled %+v err %v", reconciled, err)
	}
	if a.Schema != next || r.Schema != next || reconciled.Schema != next {
		t.Errorf("in memory the attempt says schema %d, the receipt %d and the reconciled receipt %d, want %d",
			a.Schema, r.Schema, reconciled.Schema, next)
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
		{attemptFile, readFile(t, filepath.Join(dir, attemptFile)), next},
		{receiptFile, readFile(t, filepath.Join(dir, receiptFile)), next},
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
