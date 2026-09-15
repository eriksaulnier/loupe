package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test's working directory")
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPluginManifest(t *testing.T) {
	var manifest struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "plugin/.claude-plugin/plugin.json")), &manifest); err != nil {
		t.Fatalf("plugin.json: %v", err)
	}
	if manifest.Name != "loupe" {
		t.Errorf("plugin.json name = %q, want loupe", manifest.Name)
	}
	if manifest.Version == "" {
		t.Error("plugin.json has no version")
	}
	if manifest.Description == "" {
		t.Error("plugin.json has no description")
	}
}

func TestPluginMarketplace(t *testing.T) {
	var marketplace struct {
		Name  string `json:"name"`
		Owner *struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, ".claude-plugin/marketplace.json")), &marketplace); err != nil {
		t.Fatalf("marketplace.json: %v", err)
	}
	if marketplace.Name == "" {
		t.Error("marketplace.json has no name")
	}
	if marketplace.Owner == nil || marketplace.Owner.Name == "" {
		t.Error("marketplace.json has no owner.name")
	}
	if len(marketplace.Plugins) != 1 || marketplace.Plugins[0].Name != "loupe" || marketplace.Plugins[0].Source != "./plugin" {
		t.Errorf("marketplace.json plugins = %+v, want one loupe entry with source ./plugin", marketplace.Plugins)
	}
}

func TestPluginSkill(t *testing.T) {
	skill := readRepoFile(t, "plugin/skills/loupe/SKILL.md")
	rest, ok := strings.CutPrefix(skill, "---\n")
	if !ok {
		t.Fatal("SKILL.md does not open with --- on line 1")
	}
	frontmatter, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		t.Fatal("SKILL.md frontmatter is not closed")
	}
	fields := map[string]string{}
	for line := range strings.SplitSeq(frontmatter, "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok && !strings.HasPrefix(line, " ") {
			fields[key] = strings.TrimSpace(value)
		}
	}
	if fields["name"] != "loupe" {
		t.Errorf("SKILL.md name = %q, want loupe", fields["name"])
	}
	if n := len([]rune(fields["description"])); n == 0 || n >= 1536 {
		t.Errorf("SKILL.md description is %d characters, want 1 to 1535", n)
	}

	for _, phrase := range []string{
		"loupe capture", "loupe show --previous", "git show", "loupe add --from", "loupe summary --expect-findings",
		"loupe feedback", "loupe reply", "loupe review", "loupe wait --run", "`awaiting`", "`timeout`",
		"MUST NOT run `loupe review`", "`loupe publish`", "pseudo-terminal", "pipe or script confirmation",
		"`gh pr review`", "`gh api`", "GitHub MCP",
		"`HERDR_ENV`", "herdr pane layout", "herdr pane split", "--focus", "herdr pane run", "&& exit",
		"\"loupe review '<ref>' && exit\"", "--env \"LOUPE_HOME=$LOUPE_HOME\"", "`pane_id` is `$HERDR_PANE_ID`",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("SKILL.md body does not contain %q", phrase)
		}
	}
}

// The prohibitions are pinned as whole lines, so weakening MUST NOT or dropping a route fails.
func TestPluginSkillProhibitions(t *testing.T) {
	skill := readRepoFile(t, "plugin/skills/loupe/SKILL.md")
	lines := strings.Split(skill, "\n")
	for _, want := range []string{
		"- You MUST NOT run `loupe publish` or open it for the human, in a Herdr pane or by any other route. It is human-only.",
		"- You MUST NOT run `loupe review` yourself; inside Herdr you MAY open it for the human as section 6 describes.",
		"- You MUST NOT allocate a pseudo-terminal to reach `loupe review` or `loupe publish`: no `script`, `expect`, `unbuffer`, or `pty` libraries. The Herdr pane in section 6 is the only exception.",
		"- You MUST NOT send keys or text to, read output from, resize, or close a pane running `loupe review` or `loupe publish`. The `herdr pane run` that starts review in section 6 is the only text you send it.",
		"- You MUST NOT pipe or script confirmation into any loupe command.",
		"- You MUST NOT create GitHub reviews or review comments by any other route, including `gh pr review`, `gh api`, and the GitHub MCP.",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("SKILL.md lacks the line %q", want)
		}
	}
}

// Outside Herdr, and whenever a split fails, the handoff MUST stay `loupe review` in the user's own terminal.
func TestPluginSkillHandoff(t *testing.T) {
	lines := strings.Split(readRepoFile(t, "plugin/skills/loupe/SKILL.md"), "\n")
	for _, want := range []string{
		"Tell the user to run `loupe review <ref>` in their own terminal, then block on `loupe wait --run <ref> --json`. It returns when the human quits review leaving notes for you, or when the run is published.",
		"Open a new split at every handoff. You MUST NOT look for, reuse, or close an earlier review pane. A step fails when it exits non-zero or its result lacks the field you need; do not retry it. If any step fails, tell the user the error in one line and hand off as above instead.",
	} {
		if !slices.Contains(lines, want) {
			t.Errorf("SKILL.md lacks the line %q", want)
		}
	}
}

func TestPluginCommand(t *testing.T) {
	command := readRepoFile(t, "plugin/commands/loupe.md")
	for _, phrase := range []string{"$ARGUMENTS", "loupe:loupe", "argument-hint:"} {
		if !strings.Contains(command, phrase) {
			t.Errorf("commands/loupe.md does not contain %q", phrase)
		}
	}
}
