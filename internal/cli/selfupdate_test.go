package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// releaseServer serves release files by their path under /releases/, as github.com does.
type releaseServer struct {
	files map[string][]byte
	url   string
}

func newReleaseServer(t *testing.T) *releaseServer {
	t.Helper()
	r := &releaseServer{files: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, ok := r.files[strings.TrimPrefix(req.URL.Path, "/releases/")]
		if !ok {
			http.NotFound(w, req)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	r.url = srv.URL + "/releases"
	return r
}

func platformArchive(version string) string {
	return fmt.Sprintf("loupe_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
}

// publish makes version the latest release, with a checksums.txt that covers script and this platform's archive.
func (r *releaseServer) publish(version, script string) {
	sum := sha256.Sum256([]byte(script))
	sums := fmt.Sprintf("%s  install.sh\n%s  %s\n", hex.EncodeToString(sum[:]), strings.Repeat("0", 64), platformArchive(version))
	r.files["download/v"+version+"/install.sh"] = []byte(script)
	r.files["download/v"+version+"/checksums.txt"] = []byte(sums)
	r.files["latest/download/checksums.txt"] = []byte(sums)
}

// stubScript records the variables it ran with in out, prints a line and exits with status.
func stubScript(out string, status int) string {
	return fmt.Sprintf("#!/bin/sh\nprintf '%%s|%%s|%%s\\n' \"$LOUPE_VERSION\" \"$LOUPE_INSTALL_DIR\" \"${LOUPE_UNINSTALL:-}\" > '%s'\necho 'stub script ran'\nexit %d\n", out, status)
}

type selfUpdateFixture struct {
	releases *releaseServer
	deps     Deps
	streams  *streams
	// dir is the resolved directory of the running binary.
	dir string
	// ran is where the stub script records its variables.
	ran     string
	cleanup func()
}

// newSelfUpdateFixture runs loupe as version from a symlink, so a test sees the symlink resolved.
func newSelfUpdateFixture(t *testing.T, version string) *selfUpdateFixture {
	t.Helper()
	root := t.TempDir()
	real := filepath.Join(root, "real")
	link := filepath.Join(root, "link")
	for _, dir := range []string{real, link} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExecutable(t, filepath.Join(real, "loupe"), standInBinary(version))
	if err := os.Symlink(filepath.Join(real, "loupe"), filepath.Join(link, "loupe")); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	// uninstall looks for loupe on PATH, so a developer's own install MUST NOT reach the test.
	t.Setenv("PATH", "/usr/bin:/bin")
	f := &selfUpdateFixture{releases: newReleaseServer(t), dir: resolved, ran: filepath.Join(root, "ran")}
	f.deps, f.streams = testDeps(t, map[string]string{"LOUPE_HOME": filepath.Join(root, "home")})
	f.deps.Version = version
	f.deps.Releases = f.releases.url
	f.deps.Executable = func() (string, error) { return filepath.Join(link, "loupe"), nil }
	return f
}

// skipAsRoot skips a test that relies on a directory being unwritable, which it never is for root.
func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes into any directory")
	}
}

func (f *selfUpdateFixture) ranWith(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(f.ran)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(string(data), "\n")
}

func TestUpdateRunsTheLatestReleasesScript(t *testing.T) {
	f := newSelfUpdateFixture(t, "0.21.0")
	f.releases.publish("0.22.0", stubScript(f.ran, 0))

	code := Execute(f.deps, []string{"update", "--json"})
	if code != 0 {
		t.Fatalf("exit %d; stdout %q stderr %q", code, f.streams.stdout.String(), f.streams.stderr.String())
	}
	env := decodeOne(t, f.streams.stdout.Bytes())
	if env["command"] != "update" || env["from"] != "0.21.0" || env["to"] != "0.22.0" {
		t.Errorf("result = %v", env)
	}
	if got, want := f.ranWith(t), "0.22.0|"+f.dir+"|"; got != want {
		t.Errorf("script ran with %q, want %q", got, want)
	}
	if !strings.Contains(f.streams.stderr.String(), "stub script ran") {
		t.Errorf("stderr does not carry the script's output: %q", f.streams.stderr.String())
	}
}

func TestUpdateRunsTheScriptWhenAlreadyCurrent(t *testing.T) {
	f := newSelfUpdateFixture(t, "0.22.0")
	f.releases.publish("0.22.0", stubScript(f.ran, 0))
	if code := Execute(f.deps, []string{"update"}); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, f.streams.stderr.String())
	}
	if f.ranWith(t) == "" {
		t.Error("the script did not run")
	}
	if !strings.Contains(f.streams.stdout.String(), "0.22.0") {
		t.Errorf("stdout = %q", f.streams.stdout.String())
	}
}

func TestUpdateRefusals(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		setup      func(f *selfUpdateFixture)
		code       refusal.Code
		message    string
		reason     string
		unwritable bool
	}{
		{name: "dev build", version: "dev", code: refusal.UpdateUnsupported, message: "not a release", reason: "not-release"},
		{name: "pre-release", version: "0.22.0-rc.1", code: refusal.UpdateUnsupported, message: "not a release", reason: "not-release"},
		{name: "mise install", version: "0.21.0", code: refusal.UpdateUnsupported, message: "installed by mise", reason: "mise", setup: func(f *selfUpdateFixture) {
			dir := filepath.Join(f.dir, ".local", "share", "mise", "installs", "loupe", "0.21.0")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				panic(err)
			}
			path := filepath.Join(dir, "loupe")
			if err := os.WriteFile(path, nil, 0o755); err != nil {
				panic(err)
			}
			f.deps.Executable = func() (string, error) { return path, nil }
		}},
		{name: "mise data dir elsewhere", version: "0.21.0", code: refusal.UpdateUnsupported, message: "installed by mise", reason: "mise", setup: func(f *selfUpdateFixture) {
			dir := filepath.Join(f.dir, ".mise", "installs", "github-eriksaulnier-loupe", "0.21.0")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				panic(err)
			}
			path := filepath.Join(dir, "loupe")
			if err := os.WriteFile(path, nil, 0o755); err != nil {
				panic(err)
			}
			f.deps.Executable = func() (string, error) { return path, nil }
		}},
		{name: "directory not writable", version: "0.21.0", code: refusal.UpdateUnsupported, message: "cannot write", reason: "not-writable", unwritable: true, setup: func(f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 0))
			if err := os.Chmod(f.dir, 0o555); err != nil {
				panic(err)
			}
			f.cleanup = func() { _ = os.Chmod(f.dir, 0o755) }
		}},
		{name: "no release", version: "0.21.0", code: refusal.Download, message: "HTTP 404"},
		{name: "no archive for this platform", version: "0.21.0", code: refusal.Download, message: "names no archive", setup: func(f *selfUpdateFixture) {
			f.releases.files["latest/download/checksums.txt"] = []byte(strings.Repeat("0", 64) + "  loupe_0.22.0_plan9_mips.tar.gz\n")
		}},
		{name: "script not covered", version: "0.21.0", code: refusal.Download, message: "does not cover install.sh", setup: func(f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 0))
			sums := strings.Repeat("0", 64) + "  " + platformArchive("0.22.0") + "\n"
			f.releases.files["download/v0.22.0/checksums.txt"] = []byte(sums)
		}},
		{name: "checksum mismatch", version: "0.21.0", code: refusal.Download, message: "does not match", setup: func(f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 0))
			f.releases.files["download/v0.22.0/install.sh"] = []byte("#!/bin/sh\necho tampered\n")
		}},
		{name: "script fails", version: "0.21.0", code: refusal.InstallFailed, message: "exited 3", setup: func(f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 3))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.unwritable {
				skipAsRoot(t)
			}
			f := newSelfUpdateFixture(t, c.version)
			if c.setup != nil {
				c.setup(f)
			}
			if f.cleanup != nil {
				t.Cleanup(f.cleanup)
			}
			errObj := wantCode(t, Execute(f.deps, []string{"update", "--json"}), f.streams, c.code)
			if msg, _ := errObj["message"].(string); !strings.Contains(msg, c.message) {
				t.Errorf("message %q does not say %q", msg, c.message)
			}
			if fix, _ := errObj["fix"].(string); fix == "" {
				t.Error("the refusal names no fix")
			}
			details, _ := errObj["details"].(map[string]any)
			if c.reason != "" && details["reason"] != c.reason {
				t.Errorf("details = %v, want reason %s", details, c.reason)
			}
			if c.code == refusal.InstallFailed && details["exitStatus"] != float64(3) {
				t.Errorf("details = %v, want exitStatus 3", details)
			}
			if c.code != refusal.InstallFailed && f.ranWith(t) != "" {
				t.Error("the script ran despite the refusal")
			}
		})
	}
}

func TestUninstallRunsItsOwnReleasesScript(t *testing.T) {
	f := newSelfUpdateFixture(t, "0.21.0")
	f.releases.publish("0.21.0", stubScript(f.ran, 0))
	f.releases.publish("0.22.0", "#!/bin/sh\necho 'the latest script MUST NOT run'\nexit 9\n")

	code := Execute(f.deps, []string{"uninstall", "--json"})
	if code != 0 {
		t.Fatalf("exit %d; stdout %q stderr %q", code, f.streams.stdout.String(), f.streams.stderr.String())
	}
	if got, want := f.ranWith(t), "|"+f.dir+"|1"; got != want {
		t.Errorf("script ran with %q, want %q", got, want)
	}
	env := decodeOne(t, f.streams.stdout.Bytes())
	if env["command"] != "uninstall" || env["version"] != nil {
		t.Errorf("result = %v", env)
	}
	if env["removed"] != "0.21.0" || env["dataRoot"] != f.deps.Getenv("LOUPE_HOME") {
		t.Errorf("result = %v, want removed 0.21.0 and the data root", env)
	}
}

func TestUninstallSaysWhereTheRunsStay(t *testing.T) {
	f := newSelfUpdateFixture(t, "0.21.0")
	f.releases.publish("0.21.0", stubScript(f.ran, 0))
	if code := Execute(f.deps, []string{"uninstall"}); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, f.streams.stderr.String())
	}
	if out := f.streams.stdout.String(); !strings.Contains(out, f.deps.Getenv("LOUPE_HOME")) {
		t.Errorf("stdout does not name the data root: %q", out)
	}
}

func TestUninstallRefusals(t *testing.T) {
	t.Run("dev build", func(t *testing.T) {
		f := newSelfUpdateFixture(t, "dev")
		errObj := wantCode(t, Execute(f.deps, []string{"uninstall", "--json"}), f.streams, refusal.UpdateUnsupported)
		if details, _ := errObj["details"].(map[string]any); details["reason"] != "not-release" {
			t.Errorf("details = %v", details)
		}
	})
	t.Run("mise install", func(t *testing.T) {
		f := newSelfUpdateFixture(t, "0.21.0")
		dir := filepath.Join(f.dir, "mise", "installs", "loupe", "0.21.0")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "loupe")
		writeExecutable(t, path, "")
		f.deps.Executable = func() (string, error) { return path, nil }
		errObj := wantCode(t, Execute(f.deps, []string{"uninstall", "--json"}), f.streams, refusal.UpdateUnsupported)
		if fix, _ := errObj["fix"].(string); !strings.Contains(fix, "mise unuse") {
			t.Errorf("fix = %q, want mise unuse", fix)
		}
	})
	t.Run("directory not writable", func(t *testing.T) {
		skipAsRoot(t)
		f := newSelfUpdateFixture(t, "0.21.0")
		if err := os.Chmod(f.dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(f.dir, 0o755) })
		errObj := wantCode(t, Execute(f.deps, []string{"uninstall", "--json"}), f.streams, refusal.UpdateUnsupported)
		fix, _ := errObj["fix"].(string)
		if !strings.Contains(fix, "remove "+filepath.Join(f.dir, "loupe")) || strings.Contains(fix, "reinstall") {
			t.Errorf("fix = %q, want removing the binary, not reinstalling", fix)
		}
	})
	t.Run("script fails", func(t *testing.T) {
		f := newSelfUpdateFixture(t, "0.21.0")
		f.releases.publish("0.21.0", stubScript(f.ran, 1))
		wantCode(t, Execute(f.deps, []string{"uninstall", "--json"}), f.streams, refusal.InstallFailed)
	})
}

// An exported LOUPE_UNINSTALL MUST NOT turn an update into an uninstall.
func TestUpdateClearsAnInheritedUninstall(t *testing.T) {
	t.Setenv("LOUPE_UNINSTALL", "1")
	f := newSelfUpdateFixture(t, "0.21.0")
	f.releases.publish("0.22.0", stubScript(f.ran, 0))
	if code := Execute(f.deps, []string{"update"}); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, f.streams.stderr.String())
	}
	if got, want := f.ranWith(t), "0.22.0|"+f.dir+"|"; got != want {
		t.Errorf("script ran with %q, want %q", got, want)
	}
}

// A symlink on PATH to the removed binary would be left dangling, so uninstall removes it too.
func TestUninstallRemovesASymlinkLeftDangling(t *testing.T) {
	f := newSelfUpdateFixture(t, "0.21.0")
	link, err := f.deps.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(link)+":/usr/bin:/bin")
	f.releases.publish("0.21.0", "#!/bin/sh\nrm -f \"$LOUPE_INSTALL_DIR/loupe\"\n")
	if code := Execute(f.deps, []string{"uninstall"}); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, f.streams.stderr.String())
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Errorf("the symlink %s is still there: %v", link, err)
	}
	if !strings.Contains(f.streams.stderr.String(), "Removed the symlink "+link) {
		t.Errorf("stderr does not name the symlink: %q", f.streams.stderr.String())
	}
}

// A symlink that still reaches a file is not loupe's to remove.
func TestUninstallKeepsASymlinkThatStillResolves(t *testing.T) {
	f := newSelfUpdateFixture(t, "0.21.0")
	link, err := f.deps.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(link)+":/usr/bin:/bin")
	f.releases.publish("0.21.0", stubScript(f.ran, 0))
	if code := Execute(f.deps, []string{"uninstall"}); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, f.streams.stderr.String())
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("the symlink was removed: %v", err)
	}
}

func TestReleasesDefaultsToGitHub(t *testing.T) {
	if got := (Deps{}).releases(); got != DefaultReleases {
		t.Errorf("releases = %q", got)
	}
}

// Each case reaches one failure of finding, downloading or running the script, and names where it stopped.
func TestSelfUpdateFailures(t *testing.T) {
	cases := []struct {
		name    string
		command string
		setup   func(t *testing.T, f *selfUpdateFixture)
		message string
	}{
		{name: "no running binary", command: "update", message: "find the running loupe", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.deps.Executable = func() (string, error) { return "", fmt.Errorf("no such process") }
		}},
		{name: "running binary gone", command: "update", message: "resolve the running loupe", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.deps.Executable = func() (string, error) { return filepath.Join(f.dir, "gone"), nil }
		}},
		{name: "the test binary by default", command: "update", message: "is not named loupe", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.deps.Executable = nil
		}},
		{name: "a release without its own checksums", command: "update", message: "v0.22.0/checksums.txt: HTTP 404", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 0))
			delete(f.releases.files, "download/v0.22.0/checksums.txt")
		}},
		{name: "a script the checksums name but the release lacks", command: "update", message: "install.sh: HTTP 404", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 0))
			delete(f.releases.files, "download/v0.22.0/install.sh")
		}},
		{name: "an address that does not parse", command: "update", message: "build the request", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.deps.Releases = "%zz"
		}},
		{name: "a server that is not there", command: "update", message: "could not download", setup: func(t *testing.T, f *selfUpdateFixture) {
			srv := httptest.NewServer(http.NotFoundHandler())
			srv.Close()
			f.deps.Releases = srv.URL + "/releases"
		}},
		{name: "a body cut short", command: "update", message: "could not download", setup: func(t *testing.T, f *selfUpdateFixture) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Length", "100")
				_, _ = w.Write([]byte("short"))
			}))
			t.Cleanup(srv.Close)
			f.deps.Releases = srv.URL + "/releases"
		}},
		{name: "no sh", command: "update", message: "run the install script", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.releases.publish("0.22.0", stubScript(f.ran, 0))
			t.Setenv("PATH", "")
		}},
		{name: "uninstall with no data root", command: "uninstall", message: "data root", setup: func(t *testing.T, f *selfUpdateFixture) {
			f.deps.Getenv = func(string) string { return "" }
		}},
		{name: "uninstall with no release", command: "uninstall", message: "HTTP 404"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newSelfUpdateFixture(t, "0.21.0")
			if c.setup != nil {
				c.setup(t, f)
			}
			if code := Execute(f.deps, []string{c.command, "--json"}); code == 0 {
				t.Fatalf("exit 0, want a failure; stdout %q", f.streams.stdout.String())
			}
			if out := f.streams.stdout.String() + f.streams.stderr.String(); !strings.Contains(out, c.message) {
				t.Errorf("output does not say %q:\n%s", c.message, out)
			}
		})
	}
}

// A loupe on PATH that is not a symlink to the target is someone else's.
func TestLinkOnPathOnlyNamesASymlinkToTheTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target", "loupe")
	other := filepath.Join(root, "other", "loupe")
	for _, path := range []string{target, other} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeExecutable(t, path, standInBinary("0.21.0"))
	}
	resolvedTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}
	for name, setup := range map[string]func(dir string) (want bool){
		"no loupe":         func(string) bool { return false },
		"a regular file":   func(dir string) bool { writeExecutable(t, filepath.Join(dir, "loupe"), ""); return false },
		"a link elsewhere": func(dir string) bool { mustSymlink(t, other, filepath.Join(dir, "loupe")); return false },
		"a link to target": func(dir string) bool { mustSymlink(t, target, filepath.Join(dir, "loupe")); return true },
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			want := setup(dir)
			t.Setenv("PATH", dir)
			got := linkOnPath(resolvedTarget)
			if want && got != filepath.Join(dir, "loupe") || !want && got != "" {
				t.Errorf("linkOnPath = %q, want a link: %v", got, want)
			}
		})
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

// A dangling symlink loupe cannot remove is named, and the uninstall still succeeds, since the binary is gone.
func TestUninstallWarnsWhenTheSymlinkCannotGo(t *testing.T) {
	skipAsRoot(t)
	f := newSelfUpdateFixture(t, "0.21.0")
	link, err := f.deps.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(link)+":/usr/bin:/bin")
	if err := os.Chmod(filepath.Dir(link), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(link), 0o755) })
	f.releases.publish("0.21.0", "#!/bin/sh\nrm -f \"$LOUPE_INSTALL_DIR/loupe\"\n")
	if code := Execute(f.deps, []string{"uninstall"}); code != 0 {
		t.Fatalf("exit %d; stderr %q", code, f.streams.stderr.String())
	}
	if !strings.Contains(f.streams.stderr.String(), "warning: could not remove the symlink "+link) {
		t.Errorf("stderr does not warn about the symlink: %q", f.streams.stderr.String())
	}
}

// The script replaces and removes <dir>/loupe, so a binary under another name would be left running or in place.
func TestSelfUpdateRefusesARenamedBinary(t *testing.T) {
	for command, wantFix := range map[string]string{"update": "curl", "uninstall": "by hand"} {
		t.Run(command, func(t *testing.T) {
			f := newSelfUpdateFixture(t, "0.21.0")
			f.releases.publish("0.21.0", stubScript(f.ran, 0))
			renamed := filepath.Join(f.dir, "loupe-0.21.0")
			writeExecutable(t, renamed, standInBinary("0.21.0"))
			f.deps.Executable = func() (string, error) { return renamed, nil }
			errObj := wantCode(t, Execute(f.deps, []string{command, "--json"}), f.streams, refusal.UpdateUnsupported)
			if details, _ := errObj["details"].(map[string]any); details["reason"] != "renamed" {
				t.Errorf("details = %v, want reason renamed", details)
			}
			if fix, _ := errObj["fix"].(string); !strings.Contains(fix, wantFix) {
				t.Errorf("fix = %q, want it to name %q", fix, wantFix)
			}
			if f.ranWith(t) != "" {
				t.Error("the script ran")
			}
		})
	}
}
