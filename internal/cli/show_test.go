package cli

import (
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

func TestPrintShowEscapesDraftAndTargetText(t *testing.T) {
	d := draft.NewEmpty()
	d.Summary = "sum\x1b[2Jmary"
	d.Findings = []draft.Finding{
		{ID: "f-001", Title: "ti\x1b]52;c;x\atle\u202e", Label: "is\u202esue", Location: &draft.Location{Path: "src/\x1bx.go", Line: 3}},
	}
	target := run.Target{Title: "PR\x1b[31m title\u202e"}
	readiness := draft.ReadinessOf(d)
	var out strings.Builder
	if err := printShow(&out, run.Ref{Owner: "o", Repo: "r", Number: 1, Round: 1}, target, d, draft.Dispositions(d), readiness); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.ContainsAny(got, "\x1b\a\u202e") {
		t.Fatalf("show output carries a raw control:\n%q", got)
	}
	for _, want := range []string{`PR\u001B[31m title\u202E`, `sum\u001B[2Jmary`, `ti\u001B]52;c;x\u0007tle\u202E`, `is\u202Esue`, `src/\u001Bx.go:3`} {
		if !strings.Contains(got, want) {
			t.Errorf("show output lacks %q:\n%s", want, got)
		}
	}
}
