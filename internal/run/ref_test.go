package run

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		in   string
		want Ref
	}{
		{"owner/repo#123", Ref{Owner: "owner", Repo: "repo", Number: 123}},
		{"owner/repo#123@2", Ref{Owner: "owner", Repo: "repo", Number: 123, Round: 2}},
		{"my-org/my.repo_x#1@10", Ref{Owner: "my-org", Repo: "my.repo_x", Number: 1, Round: 10}},
	}
	for _, c := range cases {
		got, err := ParseRef(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %+v, want %+v", c.in, got, c.want)
		}
		if got.String() != c.in {
			t.Fatalf("%s: String() = %q", c.in, got.String())
		}
	}
}

func TestParseRefRefuses(t *testing.T) {
	for _, in := range []string{
		"", "owner/repo", "owner/repo#", "owner/repo#abc", "owner/repo#0", "owner/repo#-1",
		"owner/repo#1@", "owner/repo#1@0", "owner/repo#1@x", "owner#1", "a/b/c#1", "/repo#1", "owner/#1",
		"owner/repo#1@2@3", "https://github.com/owner/repo/pull/1", " owner/repo#1",
	} {
		_, err := ParseRef(in)
		r, ok := refusal.As(err)
		if !ok {
			t.Fatalf("%q: expected refusal, got %v", in, err)
		}
		if r.Code != refusal.Usage || r.Fix != "use owner/repo#123 or owner/repo#123@2" {
			t.Fatalf("%q: got %+v", in, r)
		}
	}
}
