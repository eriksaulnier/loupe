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
	f := &selfUpdateFixture{releases: newReleaseServer(t), dir: resolved, ran: filepath.Join(root, "ran")}
	f.deps, f.streams = testDeps(t, map[string]string{"LOUPE_HOME": filepath.Join(root, "home")})
	f.deps.Version = version
	f.deps.Releases = f.releases.url
	f.deps.Executable = func() (string, error) { return filepath.Join(link, "loupe"), nil }
	return f
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
		name    string
		version string
		setup   func(f *selfUpdateFixture)
		code    refusal.Code
		message string
		reason  string
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
		{name: "directory not writable", version: "0.21.0", code: refusal.UpdateUnsupported, message: "cannot write", reason: "not-writable", setup: func(f *selfUpdateFixture) {
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
	t.Run("script fails", func(t *testing.T) {
		f := newSelfUpdateFixture(t, "0.21.0")
		f.releases.publish("0.21.0", stubScript(f.ran, 1))
		wantCode(t, Execute(f.deps, []string{"uninstall", "--json"}), f.streams, refusal.InstallFailed)
	})
}
