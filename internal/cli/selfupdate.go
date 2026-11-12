package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
	"github.com/secondshift-labs/loupe/internal/updatecheck"
)

// DefaultReleases is where loupe's releases download from.
const DefaultReleases = "https://github.com/eriksaulnier/loupe/releases"

const (
	curlInstall    = "curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | sh"
	downloadLimit  = 60 * time.Second
	releaseIssues  = "https://github.com/eriksaulnier/loupe/issues"
	installAsset   = "install.sh"
	pluginRemoval  = "https://github.com/eriksaulnier/loupe/blob/main/docs/install.md#removing-the-plugin-by-hand"
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
	dir, err := installDir(deps, installFixes{
		mise: "mise use -g github:eriksaulnier/loupe@latest",
		notWritable: func(dir string) string {
			return "run it as a user who can write " + dir + ", or reinstall with " + curlInstall + " into a directory you own"
		},
		renamed: func(path string) string {
			return "remove " + path + ", then reinstall with " + curlInstall + ", which names the binary loupe"
		},
	})
	if err != nil {
		return err
	}
	releases := deps.releases()
	ctx := cmd.Context()
	latest, err := latestRelease(ctx, releases)
	if err != nil {
		return err
	}
	script, err := verifiedScript(ctx, releases, latest, "loupe update")
	if err != nil {
		return err
	}
	if err := runInstallScript(ctx, deps, script, latest, "loupe update", "LOUPE_VERSION="+latest, "LOUPE_INSTALL_DIR="+dir, "LOUPE_UNINSTALL="); err != nil {
		return err
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), invocation{}, nil, map[string]any{"from": current, "to": latest})
	}
	_, err = fmt.Fprintf(deps.Stdout, "Updated loupe from %s to %s\n", current, latest)
	return err
}

func (d Deps) releases() string {
	if d.Releases != "" {
		return d.Releases
	}
	return DefaultReleases
}

const uninstallHelp = `Remove loupe, and the loupe plugin from each of Claude Code, Codex and Pi on PATH. Review runs
stay in the data root, and uninstall prints where it is.

uninstall downloads the install.sh of this release, checks it against the release's checksums.txt,
and runs it in its uninstall mode for the directory that holds this binary. That mode downloads
nothing. A marketplace that points at a local directory is left alone. When a plugin cannot be
removed, the binary stays, so uninstall can run again. A loupe on PATH that is a symlink to the
removed binary is removed too. CI and LOUPE_NO_PLUGINS leave the plugins installed, and the
output says so. The script's output goes to stderr.

A build that is not a release, and a binary that mise installed, are refused with the command
that removes them.

Result (--json):
  {"loupe": 1, "ok": true, "command": "uninstall", "removed": "0.21.0", "dataRoot": "/home/me/.local/share/loupe"}`

func newUninstallCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:     "uninstall",
		Short:   "Remove loupe and its agent plugins, and keep the runs",
		Long:    uninstallHelp,
		Example: "  loupe uninstall",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUninstall(cmd, deps)
		},
	}
}

// runUninstall runs this release's script, not the latest one, so it removes what this release installed.
func runUninstall(cmd *cobra.Command, deps Deps) error {
	current := deps.loupeVersion()
	if !updatecheck.IsRelease(current) {
		r := refusal.New(refusal.UpdateUnsupported, fmt.Sprintf("loupe %s is not a release, so uninstall cannot tell what to remove", current), "remove the binary by hand, then the plugins as "+pluginRemoval+" shows")
		r.Details = map[string]any{"reason": "not-release"}
		return r
	}
	dir, err := installDir(deps, installFixes{
		mise: "mise unuse -g github:eriksaulnier/loupe, then remove the plugins as " + pluginRemoval + " shows",
		notWritable: func(dir string) string {
			return "remove " + filepath.Join(dir, "loupe") + " as a user who can write " + dir + ", then the plugins as " + pluginRemoval + " shows"
		},
		renamed: func(path string) string {
			return "remove " + path + " by hand, then the plugins as " + pluginRemoval + " shows"
		},
	})
	if err != nil {
		return err
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	script, err := verifiedScript(ctx, deps.releases(), current, "loupe uninstall")
	if err != nil {
		return err
	}
	link := linkOnPath(filepath.Join(dir, "loupe"))
	if err := runInstallScript(ctx, deps, script, current, "loupe uninstall", "LOUPE_UNINSTALL=1", "LOUPE_INSTALL_DIR="+dir); err != nil {
		return err
	}
	removeDangling(deps, link)
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), invocation{}, nil, map[string]any{"removed": current, "dataRoot": root})
	}
	_, err = fmt.Fprintf(deps.Stdout, "Removed loupe %s. Your review runs stay in %s\n", current, root)
	return err
}

// linkOnPath is the loupe that PATH finds when it is a symlink to target, or nothing.
func linkOnPath(target string) string {
	found, err := exec.LookPath("loupe")
	if err != nil {
		return ""
	}
	if info, err := os.Lstat(found); err != nil || info.Mode()&os.ModeSymlink == 0 {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(found); err != nil || resolved != target {
		return ""
	}
	return found
}

// removeDangling removes link once the binary it reached is gone, so the person's shell finds no loupe at all.
func removeDangling(deps Deps, link string) {
	if link == "" {
		return
	}
	if _, err := os.Stat(link); !errors.Is(err, fs.ErrNotExist) {
		return
	}
	es := deps.errStyle()
	if err := os.Remove(link); err != nil {
		_, _ = fmt.Fprintf(deps.Stderr, "%s could not remove the symlink %s: %v\n", es.Emphasis(es.Warn).Render("warning:"), link, err)
		return
	}
	_, _ = fmt.Fprintf(deps.Stderr, "Removed the symlink %s\n", link)
}

// installFixes are the fixes each command gives for an install it cannot act on.
type installFixes struct {
	mise        string
	notWritable func(dir string) string
	renamed     func(path string) string
}

// installDir is the directory of the file that runs, through any symlink, so the script replaces or removes that file.
func installDir(deps Deps, fixes installFixes) (string, error) {
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
	// mise keeps each version in its own directory and links the active one, so the script would act on a copy mise
	// does not manage. MISE_DATA_DIR moves the root, so the tool's own directory name is checked too.
	if strings.Contains(path, "/mise/installs/") || strings.Contains(path, "-eriksaulnier-loupe/") {
		r := refusal.New(refusal.UpdateUnsupported, fmt.Sprintf("loupe at %s was installed by mise", path), fixes.mise)
		r.Details = map[string]any{"reason": "mise"}
		return "", r
	}
	// The script replaces and removes <dir>/loupe, so a binary under any other name would stay as it is.
	if filepath.Base(path) != "loupe" {
		r := refusal.New(refusal.UpdateUnsupported, fmt.Sprintf("the running binary %s is not named loupe, so the install script would not touch it", path), fixes.renamed(path))
		r.Details = map[string]any{"reason": "renamed"}
		return "", r
	}
	dir := filepath.Dir(path)
	// The script refuses an unwritable directory with a fix that names LOUPE_INSTALL_DIR, which update and uninstall
	// set themselves, so the check runs here first.
	if err := unix.Access(dir, unix.W_OK); err != nil {
		r := refusal.New(refusal.UpdateUnsupported, fmt.Sprintf("loupe cannot write %s, which holds the running binary: %v", dir, err), fixes.notWritable(dir))
		r.Details = map[string]any{"reason": "not-writable"}
		return "", r
	}
	return dir, nil
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

// retry is the command to run again after a refusal.
func verifiedScript(ctx context.Context, releases, version, retry string) ([]byte, error) {
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
		return nil, refusal.New(refusal.Download, fmt.Sprintf("the %s of loupe %s does not cover %s", checksumsAsset, version, installAsset), "check the release's files at "+releases+"/tag/v"+version)
	}
	script, err := download(ctx, base+installAsset, releases)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(script)
	if hex.EncodeToString(got[:]) != want {
		return nil, refusal.New(refusal.Download, fmt.Sprintf("%s of loupe %s does not match its %s", installAsset, version, checksumsAsset), "run "+retry+" again. If it fails again, report it at "+releaseIssues)
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

func runInstallScript(ctx context.Context, deps Deps, script []byte, version, retry string, env ...string) error {
	// sh reads the script from stdin, as under curl | sh. The script calls main on its last line, so sh has read all
	// of it before any command that could read stdin starts.
	c := exec.CommandContext(ctx, "sh", "-s")
	c.Env = append(os.Environ(), env...)
	c.Stdin = bytes.NewReader(script)
	c.Stdout, c.Stderr = deps.Stderr, deps.Stderr
	if err := c.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return fmt.Errorf("run the install script: %w", err)
		}
		r := refusal.New(refusal.InstallFailed, fmt.Sprintf("the install script for loupe %s exited %d", version, exit.ExitCode()), "read the script's output above, then run "+retry+" again")
		r.Details = map[string]any{"exitStatus": exit.ExitCode()}
		return r
	}
	return nil
}
