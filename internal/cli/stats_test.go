package cli

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/stats"
)

// statsFinding is an included general finding filed by the agent.
func statsFinding(id, label string, blocking bool) draft.Finding {
	return draft.Finding{ID: id, Rev: 1, Title: id, Body: "b", General: true, Label: label, Blocking: blocking,
		By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}}
}

// statsWithdrawn is a finding by withdrew with loupe edit --exclude.
func statsWithdrawn(id, by string) draft.Finding {
	f := statsFinding(id, "suggestion", false)
	f.Rev, f.Included = 2, false
	f.History = []draft.HistoryEntry{{By: by, At: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Changed: map[string]any{"included": true}}}
	return f
}

type statsSeed struct {
	owner, repo   string
	number, round int
	capturedAt    time.Time
	findings      []draft.Finding
	accept        []string
	exclude       []string
	// viewer is the receipt's; "" with a posted time is an unattended publication.
	viewer string
	posted time.Duration
	// sent overrides the envelope's findings; nil sends the accepted findings, or every included one unattended.
	sent []string
	// drift adds a finding after publication.
	drift bool
}

func seedStats(t *testing.T, home string, seeds ...statsSeed) {
	t.Helper()
	for _, s := range seeds {
		d := draft.NewEmpty()
		d.Findings = s.findings
		at := s.capturedAt.Add(time.Minute)
		for _, id := range s.accept {
			if _, err := draft.Accept(d, id, at); err != nil {
				t.Fatal(err)
			}
		}
		for _, id := range s.exclude {
			if _, err := draft.Exclude(d, id, at); err != nil {
				t.Fatal(err)
			}
		}
		d.Version = 2 + len(s.accept) + len(s.exclude)
		var receipt *publish.Receipt
		if s.posted > 0 {
			sent := s.sent
			if sent == nil {
				for _, f := range d.Findings {
					if disp := draft.Dispositions(d)[f.ID]; disp == draft.DispositionAccepted || s.viewer == "" && disp == draft.DispositionPending {
						sent = append(sent, f.ID)
					}
				}
			}
			env := publish.Envelope{Target: publish.EnvelopeTarget{Owner: s.owner, Repo: s.repo, Number: s.number, Round: s.round},
				Viewer: s.viewer, Action: "comment", Event: "COMMENT", DraftVersion: d.Version, Comments: []publish.Comment{},
				Findings: []publish.EnvelopeFinding{}}
			for _, id := range sent {
				for _, f := range d.Findings {
					if f.ID == id {
						env.Findings = append(env.Findings, publish.EnvelopeFinding{ID: id, Title: f.Title, Body: f.Body, Label: f.Label, Blocking: f.Blocking})
					}
				}
			}
			receipt = &publish.Receipt{ReviewID: int64(s.number*10 + s.round), ReviewURL: testReviewURL,
				Action: "comment", PostedAt: s.capturedAt.Add(s.posted), Envelope: env}
		}
		if s.drift {
			d.Findings = append(d.Findings, statsFinding("f-099", "question", false))
			d.Version++
		}
		writeRun(t, home, s.owner, s.repo, s.number, s.round, "stats", s.capturedAt, d)
		if receipt != nil {
			if err := publish.SaveReceipt(run.RunDir(home, s.owner, s.repo, s.number, s.round), receipt); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func runStatsJSON(t *testing.T, home string, args ...string) (map[string]json.RawMessage, *streams, int) {
	t.Helper()
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	code := Execute(deps, append([]string{"stats", "--json"}, args...))
	var env map[string]json.RawMessage
	if code == 0 {
		if err := json.Unmarshal(s.stdout.Bytes(), &env); err != nil {
			t.Fatalf("stdout is not one JSON object: %v\n%s", err, s.stdout.String())
		}
	}
	return env, s, code
}

// statsCounts is the part of a group a test compares; the median stays a string.
type statsCounts struct {
	Key          string         `json:"key"`
	Runs         stats.Runs     `json:"runs"`
	PullRequests int            `json:"pullRequests"`
	Findings     stats.Findings `json:"findings"`
	Precision    *float64       `json:"precision"`
	Median       *string        `json:"medianCaptureToPublish"`
}

func decodeTotal(t *testing.T, env map[string]json.RawMessage) statsCounts {
	t.Helper()
	var g statsCounts
	if err := json.Unmarshal(env["total"], &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// fileStates is every file under root with its bytes and modification time.
func fileStates(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = info.ModTime().String() + "\x00" + string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var statsOctober = time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

func TestStatsCoversPublishedRunsByDefaultAndTouchesNothing(t *testing.T) {
	home := t.TempDir()
	seedStats(t, home,
		statsSeed{owner: "example-org", repo: "one", number: 1, round: 1, capturedAt: statsOctober,
			findings: []draft.Finding{statsFinding("f-001", "issue", true), statsFinding("f-002", "suggestion", false)},
			accept:   []string{"f-001"}, exclude: []string{"f-002"}, viewer: "reviewer", posted: 10 * time.Minute},
		statsSeed{owner: "example-org", repo: "one", number: 2, round: 1, capturedAt: statsOctober,
			findings: []draft.Finding{statsFinding("f-001", "issue", false)}, accept: []string{"f-001"}, viewer: "reviewer", posted: 20 * time.Minute},
		statsSeed{owner: "example-org", repo: "two", number: 1, round: 1, capturedAt: statsOctober,
			findings: []draft.Finding{statsFinding("f-001", "question", false)}, viewer: "", posted: time.Minute},
		statsSeed{owner: "example-org", repo: "two", number: 2, round: 1, capturedAt: statsOctober,
			findings: []draft.Finding{statsFinding("f-001", "issue", false)}, accept: []string{"f-001"}},
		// An interrupted publication: an attempt and no receipt.
		statsSeed{owner: "example-org", repo: "two", number: 3, round: 1, capturedAt: statsOctober,
			findings: []draft.Finding{statsFinding("f-001", "issue", false)}},
	)
	if err := publish.SaveAttempt(run.RunDir(home, "example-org", "two", 3, 1), &publish.Attempt{State: publish.StateInFlight}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(run.RunsDir(home), "example-org", "two", "654-round3-abandoned-backup"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := fileStates(t, home)

	env, s, code := runStatsJSON(t, home)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	if string(env["loupe"]) != "1" || string(env["ok"]) != "true" || string(env["command"]) != `"stats"` {
		t.Fatalf("envelope %s", s.stdout.String())
	}
	for _, key := range []string{"run", "dir", "version"} {
		if _, ok := env[key]; ok {
			t.Errorf("result carries %q: %s", key, s.stdout.String())
		}
	}
	total := decodeTotal(t, env)
	if total.Runs != (stats.Runs{Published: 3, Unattended: 1}) {
		t.Fatalf("runs %+v, want three published, one unattended", total.Runs)
	}
	want := stats.Findings{Filed: 4, Published: 3, Excluded: 1}
	if total.Findings != want {
		t.Fatalf("findings %+v, want %+v", total.Findings, want)
	}
	if after := fileStates(t, home); !reflect.DeepEqual(before, after) {
		t.Fatalf("stats changed the data root:\nbefore %v\nafter  %v", keys(before), keys(after))
	}

	all, _, _ := runStatsJSON(t, home, "--state", "captured", "--state", "ready", "--state", "published")
	if got := decodeTotal(t, all).Runs; got != (stats.Runs{Captured: 1, Ready: 1, Published: 3, Unattended: 1}) {
		t.Fatalf("every state covered %+v, want the interrupted run as captured and the decided one as ready", got)
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestStatsRefusesAnUnreadableRunFile(t *testing.T) {
	for _, file := range []string{"receipt.json", "draft.json"} {
		t.Run(file, func(t *testing.T) {
			home := t.TempDir()
			seedStats(t, home,
				statsSeed{owner: "example-org", repo: "one", number: 1, round: 1, capturedAt: statsOctober,
					findings: []draft.Finding{statsFinding("f-001", "issue", false)}, accept: []string{"f-001"}, viewer: "reviewer", posted: time.Minute},
				statsSeed{owner: "example-org", repo: "one", number: 2, round: 1, capturedAt: statsOctober,
					findings: []draft.Finding{statsFinding("f-001", "issue", false)}, accept: []string{"f-001"}, viewer: "reviewer", posted: time.Minute})
			path := filepath.Join(run.RunDir(home, "example-org", "one", 2, 1), file)
			if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
				t.Fatal(err)
			}
			_, s, code := runStatsJSON(t, home)
			if code != 1 {
				t.Fatalf("exit %d, want 1; stdout %s", code, s.stdout.String())
			}
			var env struct {
				OK    bool `json:"ok"`
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(s.stdout.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not one refusal: %v\n%s", err, s.stdout.String())
			}
			if env.OK || env.Error.Code != "record" || !strings.Contains(env.Error.Message, path) {
				t.Fatalf("refusal %+v, want record naming %s", env.Error, path)
			}
			if strings.Contains(s.stdout.String(), `"total"`) {
				t.Fatalf("a partial result reached stdout: %s", s.stdout.String())
			}
		})
	}
}

// statsRuns is one run in each state and one finding of each count, across two owners, three repositories and two
// months, with one unattended run and one run whose draft changed after publishing (SC-001).
func statsRuns(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	reinstated := statsFinding("f-006", "issue", false)
	reinstated.Rev = 3
	reinstated.History = []draft.HistoryEntry{
		{By: draft.ByAgent, Changed: map[string]any{"included": true}},
		{By: draft.ByHuman, Changed: map[string]any{"included": false}},
	}
	regraded := statsFinding("f-007", "issue", true)
	regraded.Rev = 2
	regraded.History = []draft.HistoryEntry{{By: draft.ByHuman, Changed: map[string]any{"label": "suggestion", "blocking": false}}}
	seedStats(t, home,
		statsSeed{owner: "example-org", repo: "example-repo", number: 12, round: 1, capturedAt: statsOctober,
			findings: []draft.Finding{statsFinding("f-001", "issue", true), statsFinding("f-002", "suggestion", false),
				statsFinding("f-003", "question", false), statsWithdrawn("f-004", draft.ByHuman), statsWithdrawn("f-005", draft.ByAgent),
				reinstated, regraded},
			accept: []string{"f-001", "f-002", "f-006", "f-007"}, exclude: []string{"f-003"}, viewer: "reviewer", posted: 14 * time.Minute},
		// Captured on October 30, published on November 2, then changed.
		statsSeed{owner: "example-org", repo: "example-repo", number: 12, round: 2, capturedAt: time.Date(2026, 10, 30, 23, 0, 0, 0, time.UTC),
			findings: []draft.Finding{statsFinding("f-001", "issue", false)}, accept: []string{"f-001"},
			viewer: "reviewer", posted: 73 * time.Hour, drift: true},
		statsSeed{owner: "example-org", repo: "widgets", number: 3, round: 1, capturedAt: time.Date(2026, 11, 5, 8, 0, 0, 0, time.UTC),
			findings: []draft.Finding{statsFinding("f-001", "issue", true), statsFinding("f-002", "suggestion", false)},
			viewer:   "", posted: 5 * time.Minute},
		statsSeed{owner: "example-org", repo: "widgets", number: 4, round: 1, capturedAt: time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC),
			findings: []draft.Finding{statsFinding("f-001", "issue", false)}, accept: []string{"f-001"}},
		statsSeed{owner: "example-org", repo: "widgets", number: 5, round: 1, capturedAt: time.Date(2026, 11, 7, 8, 0, 0, 0, time.UTC),
			findings: []draft.Finding{statsFinding("f-001", "question", false)}},
		statsSeed{owner: "other-org", repo: "tools", number: 1, round: 1, capturedAt: time.Date(2026, 10, 20, 15, 0, 0, 0, time.UTC),
			findings: []draft.Finding{statsFinding("f-001", "perf-nit", false), statsFinding("f-002", "issue", false)},
			accept:   []string{"f-002"}, exclude: []string{"f-001"}, viewer: "reviewer", posted: time.Hour + 2*time.Minute + 5*time.Second},
	)
	return home
}

// checkFiledInResult is FR-003 over the total and every group of one JSON result.
func checkFiledInResult(t *testing.T, stdout []byte) {
	t.Helper()
	var result struct {
		Groups []statsCounts `json:"groups"`
		Total  statsCounts   `json:"total"`
	}
	if err := json.Unmarshal(stdout, &result); err != nil {
		t.Fatal(err)
	}
	for _, g := range append(result.Groups, result.Total) {
		f := g.Findings
		if f.Published+f.Excluded+f.Withdrawn+f.Pending != f.Filed {
			t.Errorf("%s: %+v does not add up to filed", g.Key, f)
		}
	}
}

// TestStatsGoldens pins SC-001: the JSON result for each grouping over statsRuns, and the human table at the two
// widths that matter.
func TestStatsGoldens(t *testing.T) {
	home := statsRuns(t)
	all := []string{"--state", "captured", "--state", "ready", "--state", "published"}
	for _, group := range []string{"none", "repo", "week", "month"} {
		t.Run("json-"+group, func(t *testing.T) {
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			if code := Execute(deps, append([]string{"stats", "--json", "--group-by", group}, all...)); code != 0 {
				t.Fatalf("exit %d stderr %s", code, s.stderr.String())
			}
			checkFiledInResult(t, s.stdout.Bytes())
			checkCLIGolden(t, "stats-json-"+group+".json", s.stdout.String())
		})
	}
	empty := t.TempDir()
	cases := []struct {
		name, home string
		args       []string
	}{
		{"stats-help", home, []string{"stats", "--help"}},
		{"stats", home, []string{"stats"}},
		{"stats-repo", home, append([]string{"stats", "--group-by", "repo"}, all...)},
		{"stats-month", home, []string{"stats", "example-org/example-repo", "EXAMPLE-ORG/widgets", "--group-by", "month", "--since", "2026-11-01"}},
		{"stats-nomatch", home, []string{"stats", "nobody/nothing"}},
		{"stats-empty", empty, []string{"stats"}},
	}
	for _, width := range []int{100, 80} {
		for _, c := range cases {
			t.Run(c.name+"."+strconv.Itoa(width), func(t *testing.T) {
				deps, s := testDeps(t, map[string]string{"LOUPE_HOME": c.home, "NO_COLOR": "1", "LANG": "en_US.UTF-8"})
				deps.TermWidth = func() int { return width }
				if code := Execute(deps, c.args); code != 0 {
					t.Fatalf("exit %d stderr %s", code, s.stderr.String())
				}
				checkCLIGolden(t, c.name+"."+strconv.Itoa(width)+".txt", s.stdout.String()+s.stderr.String())
			})
		}
	}
}

func TestStatsFilterEchoesWhatIsInForce(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		args []string
		want string
	}{
		{nil, `{"repos":[],"since":null,"until":null,"states":["published"],"groupBy":"none"}`},
		{[]string{"Example-Org/One", "example-org/two", "--since", "2026-11-01", "--until", "2026-11-30",
			"--state", "published", "--state", "captured", "--state", "published", "--group-by", "week"},
			`{"repos":["Example-Org/One","example-org/two"],"since":"2026-11-01T00:00:00Z","until":"2026-11-30T23:59:59.999999999Z","states":["captured","published"],"groupBy":"week"}`},
		{[]string{"--since", "2026-11-01T12:00:00+02:00", "--until", "2026-11-01T12:00:00.5Z"},
			`{"repos":[],"since":"2026-11-01T10:00:00Z","until":"2026-11-01T12:00:00.5Z","states":["published"],"groupBy":"none"}`},
	}
	for _, c := range cases {
		env, s, code := runStatsJSON(t, home, c.args...)
		if code != 0 {
			t.Fatalf("%v: exit %d stderr %s", c.args, code, s.stderr.String())
		}
		if got := string(env["filter"]); got != c.want {
			t.Errorf("%v: filter\n%s\nwant\n%s", c.args, got, c.want)
		}
	}
}

func TestStatsRefusesBadArgumentsAsUsage(t *testing.T) {
	cases := [][]string{
		{"--since", "2026-11-01", "--until", "2026-10-01"},
		{"--since", "2026-13-01"},
		{"--until", "yesterday"},
		{"--since", ""},
		{"--until", ""},
		{"--state", "merged"},
		{"--state", ""},
		{"--group-by", "day"},
		{"example-org"},
		{"example-org/one/two"},
		{"example-org/one#3"},
		{"/one"},
	}
	for _, args := range cases {
		// No data root at all: a usage refusal comes before the walk.
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": filepath.Join(t.TempDir(), "missing")})
		code := Execute(deps, append([]string{"stats", "--json"}, args...))
		if code != 2 || !strings.Contains(s.stdout.String(), `"code":"usage"`) {
			t.Errorf("%v: exit %d stdout %s, want a usage refusal and exit 2", args, code, s.stdout.String())
		}
	}
}

// SC-003: 500 runs across 20 repositories, 20 findings each and a receipt on every second run, in under a second.
// Two runs are seeded the ordinary way and copied with plain writes, since CreateRun's fsyncs would cost the suite
// far more than the command under test.
func TestStatsReadsFiveHundredRunsInUnderASecond(t *testing.T) {
	var findings []draft.Finding
	for i := 1; i <= 20; i++ {
		findings = append(findings, statsFinding(fmt.Sprintf("f-%03d", i), "issue", false))
	}
	accept := []string{"f-001", "f-002", "f-003"}
	templates := t.TempDir()
	seedStats(t, templates,
		statsSeed{owner: "o", repo: "r", number: 1, round: 1, capturedAt: statsOctober, findings: findings, accept: accept,
			viewer: "reviewer", posted: 10 * time.Minute},
		statsSeed{owner: "o", repo: "r", number: 2, round: 1, capturedAt: statsOctober, findings: slices.Clone(findings), accept: accept})
	read := func(number int, name string) []byte {
		data, err := os.ReadFile(filepath.Join(run.RunDir(templates, "o", "r", number, 1), name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	files := map[string][]byte{"pr.diff": {}, "draft.json": read(1, "draft.json"), "receipt.json": read(1, "receipt.json")}
	unpublishedDraft := read(2, "draft.json")

	home := t.TempDir()
	for i := range 500 {
		repo, number := fmt.Sprintf("repo-%02d", i%20), i/20+1
		dir := run.RunDir(home, "example-org", repo, number, 1)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		target, err := json.Marshal(run.Target{Schema: run.TargetSchema, Owner: "example-org", Repo: repo, Number: number, Round: 1,
			CapturedAt: statsOctober.Add(time.Duration(i) * time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		write := map[string][]byte{"target.json": target, "pr.diff": files["pr.diff"], "draft.json": unpublishedDraft}
		if i%2 == 0 {
			write["draft.json"], write["receipt.json"] = files["draft.json"], files["receipt.json"]
		}
		for name, data := range write {
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	start := time.Now()
	env, s, code := runStatsJSON(t, home, "--state", "captured", "--state", "ready", "--state", "published", "--group-by", "repo")
	elapsed := time.Since(start)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	if got := decodeTotal(t, env); got.Findings.Filed != 500*20 || got.Runs.Published != 250 {
		t.Fatalf("total %+v, want 10000 findings and 250 published runs", got)
	}
	if elapsed > time.Second {
		t.Fatalf("stats took %s over 500 runs, want under 1s", elapsed)
	}
	t.Logf("stats over 500 runs: %s", elapsed)
}
