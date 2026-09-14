package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
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
		Name    string `json:"name"`
		Version string `json:"version"`
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
		"loupe feedback", "loupe reply", "loupe review",
		"MUST NOT run `loupe review`", "`loupe publish`", "pseudo-terminal", "pipe or script confirmation",
		"`gh pr review`", "`gh api`", "GitHub MCP",
	} {
		if !strings.Contains(body, phrase) {
			t.Errorf("SKILL.md body does not contain %q", phrase)
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
