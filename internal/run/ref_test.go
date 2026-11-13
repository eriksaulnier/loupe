package run

import (
	"testing"

	"github.com/secondshift-labs/loupe/internal/refusal"
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
		"owner/repo#99999999999999999999", "owner/repo#1@99999999999999999999",
	} {
		_, err := ParseRef(in)
		r, ok := refusal.As(err)
		if !ok {
			t.Fatalf("%q: expected refusal, got %v", in, err)
		}
		if r.Code != refusal.Usage || r.Fix != "use owner/repo#123 or local#<n>" {
			t.Fatalf("%q: got %+v", in, r)
		}
	}
}

func TestParseLocalRef(t *testing.T) {
	cases := []struct {
		in   string
		want Ref
	}{
		{"local#7", Ref{Local: 7}},
		{"eriksaulnier/loupe:local#7", Ref{Prefix: "eriksaulnier/loupe", Local: 7}},
		{"myrepo:local#7", Ref{Prefix: "myrepo", Local: 7}},
	}
	for _, c := range cases {
		got, err := ParseRef(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestLocalRefString(t *testing.T) {
	cases := []struct {
		ref  Ref
		want string
	}{
		{Ref{Local: 7}, "local#7"},
		{Ref{Local: 7, Prefix: "eriksaulnier/loupe"}, "eriksaulnier/loupe:local#7"},
		{Ref{Local: 7, Prefix: "myrepo"}, "myrepo:local#7"},
	}
	for _, c := range cases {
		if got := c.ref.String(); got != c.want {
			t.Fatalf("String() = %q, want %q", got, c.want)
		}
	}
}

func TestParseLocalRefRefuses(t *testing.T) {
	for _, in := range []string{"local#0", ":local#1", "a/b/c:local#1", "local#1@2", "local#99999999999999999999"} {
		_, err := ParseRef(in)
		r, ok := refusal.As(err)
		if !ok {
			t.Fatalf("%q: expected refusal, got %v", in, err)
		}
		if r.Code != refusal.Usage || r.Fix != "use owner/repo#123 or local#<n>" {
			t.Fatalf("%q: got %+v", in, r)
		}
	}
}
