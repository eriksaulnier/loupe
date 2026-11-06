package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/updatecheck"
)

// DefaultReleases is where loupe's releases download from.
const DefaultReleases = "https://github.com/eriksaulnier/loupe/releases"

const (
	curlInstall    = "curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | sh"
	downloadLimit  = 60 * time.Second
	releaseIssues  = "https://github.com/eriksaulnier/loupe/issues"
	installAsset   = "install.sh"
	checksumsAsset = "checksums.txt"
)

const selfUpdateHelp = `Install the latest loupe release, and the loupe plugin at that release for each of Claude Code,
Codex and Pi on PATH.

update reads the latest release's checksums.txt, downloads that release's install.sh, checks it
against the checksums, and runs it for the directory that holds this binary. It runs even when
this release is the latest, so a plugin that is missing or on another release is set up again.
The script's output goes to stderr. LOUPE_NO_PLUGINS skips the plugins.

A build that is not a release, and a binary that mise installed, are refused with the command
that updates them.

Result (--json):
  {"loupe": 1, "ok": true, "command": "update", "from": "0.21.0", "to": "0.22.0"}`

func newSelfUpdateCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:     "update",
		Short:   "Install the latest release and its agent plugins",
		Long:    selfUpdateHelp,
		Example: "  loupe update",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSelfUpdate(cmd, deps)
		},
	}
}

func runSelfUpdate(cmd *cobra.Command, deps Deps) error {
	current := deps.loupeVersion()
	if !updatecheck.IsRelease(current) {
		r := refusal.New(refusal.UpdateUnsupported, fmt.Sprintf("loupe %s is not a release, so update cannot tell what it would replace", current), curlInstall)
		r.Details = map[string]any{"reason": "not-release"}
		return r
	}
	dir, err := installDir(deps)
	if err != nil {
		return err
	}
	releases := deps.Releases
	if releases == "" {
		releases = DefaultReleases
	}
	ctx := cmd.Context()
	latest, err := latestRelease(ctx, releases)
	if err != nil {
		return err
	}
	script, err := verifiedScript(ctx, releases, latest)
	if err != nil {
		return err
	}
	if err := runInstallScript(ctx, deps, script, latest, dir); err != nil {
		return err
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), invocation{}, nil, map[string]any{"from": current, "to": latest})
	}
	_, err = fmt.Fprintf(deps.Stdout, "Updated loupe from %s to %s\n", current, latest)
	return err
}

// installDir is the directory of the file that runs, through any symlink, so the script replaces that file.
func installDir(deps Deps) (string, error) {
	executable := deps.Executable
	if executable == nil {
		executable = os.Executable
	}
	path, err := executable()
	if err != nil {
		return "", fmt.Errorf("find the running loupe: %w", err)
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return "", fmt.Errorf("resolve the running loupe: %w", err)
	}
	// mise keeps each version in its own directory and links the active one, so the script would add a copy mise
	// never runs.
	if strings.Contains(path, string(filepath.Separator)+filepath.Join("mise", "installs")+string(filepath.Separator)) {
		r := refusal.New(refusal.UpdateUnsupported, fmt.Sprintf("loupe at %s was installed by mise", path), "mise use -g github:eriksaulnier/loupe@latest")
		r.Details = map[string]any{"reason": "mise"}
		return "", r
	}
	return filepath.Dir(path), nil
}

// latestRelease reads the version from the archive name in the latest release's checksums, as install.sh does, so
// it needs no token and no API call.
func latestRelease(ctx context.Context, releases string) (string, error) {
	sums, err := download(ctx, releases+"/latest/download/"+checksumsAsset, releases)
	if err != nil {
		return "", err
	}
	prefix, suffix := "loupe_", "_"+runtime.GOOS+"_"+runtime.GOARCH+".tar.gz"
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasPrefix(fields[1], prefix) && strings.HasSuffix(fields[1], suffix) {
			return strings.TrimSuffix(strings.TrimPrefix(fields[1], prefix), suffix), nil
		}
	}
	return "", refusal.New(refusal.Download, fmt.Sprintf("the latest release's %s names no archive for %s_%s", checksumsAsset, runtime.GOOS, runtime.GOARCH), "check the release's files at "+releases)
}

func verifiedScript(ctx context.Context, releases, version string) ([]byte, error) {
	base := releases + "/download/v" + version + "/"
	sums, err := download(ctx, base+checksumsAsset, releases)
	if err != nil {
		return nil, err
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == installAsset {
			want = fields[0]
		}
	}
	if want == "" {
		return nil, refusal.New(refusal.Download, fmt.Sprintf("the %s of loupe %s does not cover %s", checksumsAsset, version, installAsset), curlInstall)
	}
	script, err := download(ctx, base+installAsset, releases)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(script)
	if hex.EncodeToString(got[:]) != want {
		return nil, refusal.New(refusal.Download, fmt.Sprintf("%s of loupe %s does not match its %s", installAsset, version, checksumsAsset), "run loupe update again. If it fails again, report it at "+releaseIssues)
	}
	return script, nil
}

func download(ctx context.Context, url, releases string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadLimit)
	defer cancel()
	fix := "check network access to github.com, and that the release exists at " + releases
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build the request for %s: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, refusal.New(refusal.Download, fmt.Sprintf("could not download %s: %v", url, err), fix)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, refusal.New(refusal.Download, fmt.Sprintf("could not download %s: HTTP %d", url, resp.StatusCode), fix)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, refusal.New(refusal.Download, fmt.Sprintf("could not download %s: %v", url, err), fix)
	}
	return body, nil
}

// runInstallScript pins the release, so the script installs the version whose script this is.
func runInstallScript(ctx context.Context, deps Deps, script []byte, version, dir string) error {
	tmp, err := os.MkdirTemp("", "loupe-update-")
	if err != nil {
		return fmt.Errorf("create a temporary directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	path := filepath.Join(tmp, installAsset)
	if err := os.WriteFile(path, script, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	c := exec.CommandContext(ctx, "sh", path)
	c.Env = append(os.Environ(), "LOUPE_VERSION="+version, "LOUPE_INSTALL_DIR="+dir)
	c.Stdout, c.Stderr = deps.Stderr, deps.Stderr
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return fmt.Errorf("run the install script: %w", err)
		}
		r := refusal.New(refusal.InstallFailed, fmt.Sprintf("the install script for loupe %s exited %d", version, exit.ExitCode()), "read the script's output above, or run "+curlInstall)
		r.Details = map[string]any{"exitStatus": exit.ExitCode()}
		return r
	}
	return nil
}
