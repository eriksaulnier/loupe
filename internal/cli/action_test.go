package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// Pinning the install action's sha pins the binary only while its version default is the release it shipped in.
func TestInstallAction(t *testing.T) {
	var config struct {
		Packages map[string]struct {
			ExtraFiles []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"extra-files"`
		} `json:"packages"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "release-please-config.json")), &config); err != nil {
		t.Fatalf("release-please-config.json: %v", err)
	}
	generic := false
	for _, f := range config.Packages["."].ExtraFiles {
		generic = generic || (f.Type == "generic" && f.Path == "action.yml")
	}
	if !generic {
		t.Error("release-please-config.json MUST list action.yml as a generic extra-file")
	}

	pin := "default: v" + releaseVersion(t) + " # x-release-please-version"
	if !strings.Contains(readRepoFile(t, "action.yml"), pin) {
		t.Errorf("action.yml MUST carry %q", pin)
	}
}
