package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Serves release files from $FAKE_RELEASE by the URL path after /releases/, and records each request. A missing file
// fails as curl -f does on a 404.
const fakeCurl = `#!/bin/sh
out=
while [ $# -gt 1 ]; do
	if [ "$1" = -o ]; then out=$2; fi
	shift
done
printf '%s\n' "$1" >> "$FAKE_RELEASE/requests"
path=${1#*/releases/}
[ -f "$FAKE_RELEASE/$path" ] || exit 22
cp "$FAKE_RELEASE/$path" "$out"
`

const fakeUname = `#!/bin/sh
case "$1" in
-s) printf '%s\n' "$FAKE_OS" ;;
-m) printf '%s\n' "$FAKE_ARCH" ;;
*) exit 2 ;;
esac
`

type installFixture struct {
	release string
	bin     string
	home    string
	target  string
	env     []string
}

func newInstallFixture(t *testing.T) *installFixture {
	t.Helper()
	root := t.TempDir()
	f := &installFixture{
		release: filepath.Join(root, "release"),
		bin:     filepath.Join(root, "bin"),
		home:    filepath.Join(root, "home"),
	}
	f.target = filepath.Join(f.home, ".local", "bin")
	for _, dir := range []string{f.release, f.bin, f.home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExecutable(t, filepath.Join(f.bin, "curl"), fakeCurl)
	writeExecutable(t, filepath.Join(f.bin, "uname"), fakeUname)
	f.env = []string{
		"HOME=" + f.home,
		"PATH=" + f.bin + ":/usr/bin:/bin",
		"FAKE_RELEASE=" + f.release,
		"FAKE_OS=Linux",
		"FAKE_ARCH=x86_64",
		"TMPDIR=" + root,
	}
	return f
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func standInBinary(version string) string {
	return "#!/bin/sh\necho 'loupe version " + version + "'\n"
}

// A goreleaser archive also carries the license and readme, which the script MUST leave behind.
func standInArchive(t *testing.T, binary string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, file := range []struct {
		name, content string
		mode          int64
	}{{"LICENSE", "license\n", 0o644}, {"loupe", binary, 0o755}} {
		if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: file.mode, Size: int64(len(file.content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(file.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (f *installFixture) writeRelease(t *testing.T, path string, content []byte) {
	t.Helper()
	full := filepath.Join(f.release, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *installFixture) publish(t *testing.T, version, binary string, targets ...string) (archive, digest string) {
	t.Helper()
	if len(targets) == 0 {
		targets = []string{"linux_amd64"}
	}
	data := standInArchive(t, binary)
	digest = sha256Hex(data)
	var sums strings.Builder
	for _, target := range targets {
		archive = fmt.Sprintf("loupe_%s_%s.tar.gz", version, target)
		f.writeRelease(t, "download/v"+version+"/"+archive, data)
		fmt.Fprintf(&sums, "%s  %s\n", digest, archive)
	}
	f.writeRelease(t, "download/v"+version+"/checksums.txt", []byte(sums.String()))
	return archive, digest
}

func (f *installFixture) markLatest(t *testing.T, version string) {
	t.Helper()
	sums, err := os.ReadFile(filepath.Join(f.release, "download", "v"+version, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	f.writeRelease(t, "latest/download/checksums.txt", sums)
}

func (f *installFixture) setChecksums(t *testing.T, version, content string) {
	t.Helper()
	f.writeRelease(t, "download/v"+version+"/checksums.txt", []byte(content))
}

func (f *installFixture) requests(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.release, "requests"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(data))
}

func (f *installFixture) run(t *testing.T, env ...string) (stdout, stderr string, err error) {
	t.Helper()
	return f.runScript(t, filepath.Join(repoRoot(t), "install.sh"), env...)
}

func (f *installFixture) runScript(t *testing.T, script string, env ...string) (stdout, stderr string, err error) {
	t.Helper()
	sh, lookErr := exec.LookPath("sh")
	if lookErr != nil {
		t.Fatal(lookErr)
	}
	cmd := exec.Command(sh, script)
	cmd.Dir = f.home
	// Later entries win, so a case MAY replace any variable of the fixture's.
	cmd.Env = append(append([]string{}, f.env...), env...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err = cmd.Run()
	return out.String(), errOut.String(), err
}

// mustRefuse asserts the loupe stderr shape, and that the attempt changed nothing in the target directory.
func (f *installFixture) mustRefuse(t *testing.T, wantError string, env ...string) {
	t.Helper()
	before := listDir(t, f.target)
	_, stderr, err := f.run(t, env...)
	if err == nil {
		t.Fatalf("the script exited 0, want a refusal. stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "error: ") || !strings.Contains(stderr, "\nfix: ") {
		t.Errorf("stderr has no error and fix lines:\n%s", stderr)
	}
	if !strings.Contains(stderr, wantError) {
		t.Errorf("stderr does not name %q:\n%s", wantError, stderr)
	}
	if after := listDir(t, f.target); after != before {
		t.Errorf("the target directory changed:\nbefore: %s\nafter:  %s", before, after)
	}
}

// listDir returns each entry and its content, so a comparison sees a replaced binary and a leftover temporary file.
func listDir(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return "(no directory)"
	}
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, entry := range entries {
		fmt.Fprintf(&out, "%s:", entry.Name())
		if !entry.IsDir() {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "%q ", data)
		}
	}
	return out.String()
}

func (f *installFixture) placeExisting(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll(f.target, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := standInBinary("0.0.1")
	writeExecutable(t, filepath.Join(f.target, "loupe"), existing)
	return existing
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallScriptLatest(t *testing.T) {
	f := newInstallFixture(t)
	archive, _ := f.publish(t, "1.2.3", standInBinary("1.2.3"))
	f.markLatest(t, "1.2.3")

	stdout, stderr, err := f.run(t, "PATH="+f.bin+":"+f.target+":/usr/bin:/bin")
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	if got := mustRead(t, filepath.Join(f.target, "loupe")); got != standInBinary("1.2.3") {
		t.Errorf("installed binary = %q", got)
	}
	info, err := os.Stat(filepath.Join(f.target, "loupe"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("installed binary mode = %v, want executable", info.Mode())
	}
	if want := "loupe version 1.2.3"; !strings.Contains(stdout, want) {
		t.Errorf("stdout does not print %q:\n%s", want, stdout)
	}
	if !strings.Contains(stdout, "docs/install.md") {
		t.Errorf("stdout does not point at the agent plugin:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
	// LICENSE stays out of the target, so a listing holds the binary and nothing else.
	if got, want := listDir(t, f.target), "loupe:"; !strings.HasPrefix(got, want) || strings.Count(got, ":") != 1 {
		t.Errorf("target holds %s", got)
	}

	requests := f.requests(t)
	if len(requests) != 2 {
		t.Fatalf("requests = %v, want the checksums and the archive", requests)
	}
	if !strings.HasSuffix(requests[0], "/latest/download/checksums.txt") {
		t.Errorf("first request = %s", requests[0])
	}
	// A release published after the first request MUST NOT change which archive is fetched.
	if !strings.HasSuffix(requests[1], "/download/v1.2.3/"+archive) {
		t.Errorf("second request = %s", requests[1])
	}
}

func TestInstallScriptPinnedVersion(t *testing.T) {
	for _, pin := range []string{"v1.2.3", "1.2.3"} {
		t.Run(pin, func(t *testing.T) {
			f := newInstallFixture(t)
			f.publish(t, "1.2.3", standInBinary("1.2.3"))
			f.publish(t, "9.9.9", standInBinary("9.9.9"))
			f.markLatest(t, "9.9.9")

			stdout, stderr, err := f.run(t, "LOUPE_VERSION="+pin)
			if err != nil {
				t.Fatalf("install failed: %v\n%s", err, stderr)
			}
			if want := "loupe version 1.2.3"; !strings.Contains(stdout, want) {
				t.Errorf("stdout does not print %q:\n%s", want, stdout)
			}
			for _, request := range f.requests(t) {
				if !strings.Contains(request, "/download/v1.2.3/") {
					t.Errorf("request outside the pinned release: %s", request)
				}
			}
		})
	}
}

func TestInstallScriptPlatforms(t *testing.T) {
	for _, tc := range []struct{ os, arch, target string }{
		{"Linux", "x86_64", "linux_amd64"},
		{"Linux", "amd64", "linux_amd64"},
		{"Linux", "aarch64", "linux_arm64"},
		{"Linux", "arm64", "linux_arm64"},
		{"Darwin", "x86_64", "darwin_amd64"},
		{"Darwin", "arm64", "darwin_arm64"},
	} {
		t.Run(tc.os+"/"+tc.arch, func(t *testing.T) {
			f := newInstallFixture(t)
			f.publish(t, "1.2.3", standInBinary("1.2.3"), "linux_amd64", "linux_arm64", "darwin_amd64", "darwin_arm64")
			f.markLatest(t, "1.2.3")

			_, stderr, err := f.run(t, "FAKE_OS="+tc.os, "FAKE_ARCH="+tc.arch)
			if err != nil {
				t.Fatalf("install failed: %v\n%s", err, stderr)
			}
			requests := f.requests(t)
			if want := "loupe_1.2.3_" + tc.target + ".tar.gz"; !strings.HasSuffix(requests[len(requests)-1], want) {
				t.Errorf("requests = %v, want %s", requests, want)
			}
		})
	}
}

func TestInstallScriptInstallDir(t *testing.T) {
	for _, suffix := range []string{"", "/"} {
		t.Run("suffix "+suffix, func(t *testing.T) {
			f := newInstallFixture(t)
			f.publish(t, "1.2.3", standInBinary("1.2.3"))
			f.markLatest(t, "1.2.3")
			dir := filepath.Join(f.home, "custom", "bin")

			_, stderr, err := f.run(t, "LOUPE_INSTALL_DIR="+dir+suffix, "PATH="+f.bin+":"+dir+":/usr/bin:/bin")
			if err != nil {
				t.Fatalf("install failed: %v\n%s", err, stderr)
			}
			if got := mustRead(t, filepath.Join(dir, "loupe")); got != standInBinary("1.2.3") {
				t.Errorf("installed binary = %q", got)
			}
			if _, err := os.Stat(f.target); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the default directory exists, err = %v", err)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want nothing", stderr)
			}
		})
	}
}

func TestInstallScriptReplacesExisting(t *testing.T) {
	f := newInstallFixture(t)
	f.placeExisting(t)
	f.publish(t, "1.2.3", standInBinary("1.2.3"))
	f.markLatest(t, "1.2.3")

	if _, stderr, err := f.run(t); err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	if got := mustRead(t, filepath.Join(f.target, "loupe")); got != standInBinary("1.2.3") {
		t.Errorf("installed binary = %q", got)
	}
	if got := listDir(t, f.target); strings.Count(got, ":") != 1 {
		t.Errorf("target holds %s", got)
	}
}

func TestInstallScriptRefusals(t *testing.T) {
	const version = "1.2.3"
	badDigest := strings.Repeat("0", 64)
	for _, tc := range []struct {
		name      string
		arrange   func(t *testing.T, f *installFixture)
		env       []string
		wantError string
	}{
		{
			name: "checksum mismatch",
			arrange: func(t *testing.T, f *installFixture) {
				archive, _ := f.publish(t, version, standInBinary(version))
				f.setChecksums(t, version, badDigest+"  "+archive+"\n")
				f.markLatest(t, version)
			},
			wantError: "checksum",
		},
		{
			name: "no entry for the platform",
			arrange: func(t *testing.T, f *installFixture) {
				f.publish(t, version, standInBinary(version), "darwin_arm64")
				f.markLatest(t, version)
			},
			wantError: "linux_amd64",
		},
		{
			name: "an entry that only ends with the name",
			arrange: func(t *testing.T, f *installFixture) {
				archive, digest := f.publish(t, version, standInBinary(version))
				f.setChecksums(t, version, digest+"  "+archive+".sig\n")
				f.markLatest(t, version)
			},
			wantError: "linux_amd64",
		},
		{
			name: "two entries for the platform",
			arrange: func(t *testing.T, f *installFixture) {
				archive, digest := f.publish(t, version, standInBinary(version))
				f.setChecksums(t, version, digest+"  "+archive+"\n"+digest+"  loupe_1.2.4_linux_amd64.tar.gz\n")
				f.markLatest(t, version)
			},
			wantError: "linux_amd64",
		},
		{
			name: "unsupported OS",
			arrange: func(t *testing.T, f *installFixture) {
				f.publish(t, version, standInBinary(version))
				f.markLatest(t, version)
			},
			env:       []string{"FAKE_OS=FreeBSD"},
			wantError: "FreeBSD",
		},
		{
			name: "unsupported architecture",
			arrange: func(t *testing.T, f *installFixture) {
				f.publish(t, version, standInBinary(version))
				f.markLatest(t, version)
			},
			env:       []string{"FAKE_ARCH=riscv64"},
			wantError: "riscv64",
		},
		{
			name:      "checksums download fails",
			arrange:   func(t *testing.T, f *installFixture) {},
			wantError: "checksums.txt",
		},
		{
			name: "archive download fails",
			arrange: func(t *testing.T, f *installFixture) {
				archive, _ := f.publish(t, version, standInBinary(version))
				f.markLatest(t, version)
				if err := os.Remove(filepath.Join(f.release, "download", "v"+version, archive)); err != nil {
					t.Fatal(err)
				}
			},
			wantError: "linux_amd64.tar.gz",
		},
		{
			name: "pinned release does not exist",
			arrange: func(t *testing.T, f *installFixture) {
				f.publish(t, version, standInBinary(version))
				f.markLatest(t, version)
			},
			env:       []string{"LOUPE_VERSION=v0.0.0"},
			wantError: "v0.0.0",
		},
		{
			name: "new binary does not run",
			arrange: func(t *testing.T, f *installFixture) {
				f.publish(t, version, "#!/bin/sh\necho broken >&2\nexit 3\n")
				f.markLatest(t, version)
			},
			wantError: "--version",
		},
		{
			name: "archive without the binary",
			arrange: func(t *testing.T, f *installFixture) {
				var buf bytes.Buffer
				gz := gzip.NewWriter(&buf)
				tw := tar.NewWriter(gz)
				if err := tw.WriteHeader(&tar.Header{Name: "LICENSE", Mode: 0o644, Size: 1}); err != nil {
					t.Fatal(err)
				}
				if _, err := tw.Write([]byte("x")); err != nil {
					t.Fatal(err)
				}
				if err := tw.Close(); err != nil {
					t.Fatal(err)
				}
				if err := gz.Close(); err != nil {
					t.Fatal(err)
				}
				archive := "loupe_" + version + "_linux_amd64.tar.gz"
				f.writeRelease(t, "download/v"+version+"/"+archive, buf.Bytes())
				f.setChecksums(t, version, sha256Hex(buf.Bytes())+"  "+archive+"\n")
				f.markLatest(t, version)
			},
			wantError: "extract",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newInstallFixture(t)
			existing := f.placeExisting(t)
			tc.arrange(t, f)
			f.mustRefuse(t, tc.wantError, tc.env...)
			if got := mustRead(t, filepath.Join(f.target, "loupe")); got != existing {
				t.Errorf("the existing binary changed: %q", got)
			}
		})
	}
}

func TestInstallScriptRefusesBeforeDownloadOnUnsupportedPlatform(t *testing.T) {
	f := newInstallFixture(t)
	f.mustRefuse(t, "FreeBSD", "FAKE_OS=FreeBSD")
	if got := f.requests(t); got != nil {
		t.Errorf("requests = %v, want none", got)
	}
}

func TestInstallScriptRefusesFreshTargetWithoutLeavingBinary(t *testing.T) {
	f := newInstallFixture(t)
	archive, _ := f.publish(t, "1.2.3", standInBinary("1.2.3"))
	f.setChecksums(t, "1.2.3", strings.Repeat("0", 64)+"  "+archive+"\n")
	f.markLatest(t, "1.2.3")
	f.mustRefuse(t, "checksum")
	if _, err := os.Stat(filepath.Join(f.target, "loupe")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a binary landed in the target, err = %v", err)
	}
}

func TestInstallScriptRefusesDirectoryNamedLoupe(t *testing.T) {
	f := newInstallFixture(t)
	f.publish(t, "1.2.3", standInBinary("1.2.3"))
	f.markLatest(t, "1.2.3")
	if err := os.MkdirAll(filepath.Join(f.target, "loupe"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.mustRefuse(t, "directory")
	if got := listDir(t, filepath.Join(f.target, "loupe")); got != "" {
		t.Errorf("the directory now holds %s", got)
	}
}

// Links in every tool the script needs, so that a case controls which checksum tool exists on PATH.
func (f *installFixture) restrictedPath(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tools")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"awk", "cat", "chmod", "cp", "gzip", "mkdir", "mktemp", "mv", "rm", "tar"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(dir, tool)); err != nil {
			t.Fatal(err)
		}
	}
	for _, fake := range []string{"curl", "uname"} {
		if err := os.Symlink(filepath.Join(f.bin, fake), filepath.Join(dir, fake)); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInstallScriptChecksumTools(t *testing.T) {
	t.Run("neither exists", func(t *testing.T) {
		f := newInstallFixture(t)
		f.publish(t, "1.2.3", standInBinary("1.2.3"))
		f.markLatest(t, "1.2.3")
		f.mustRefuse(t, "sha256sum", "PATH="+f.restrictedPath(t))
	})

	t.Run("falls back to shasum", func(t *testing.T) {
		f := newInstallFixture(t)
		_, digest := f.publish(t, "1.2.3", standInBinary("1.2.3"))
		f.markLatest(t, "1.2.3")
		dir := f.restrictedPath(t)
		writeExecutable(t, filepath.Join(dir, "shasum"),
			"#!/bin/sh\n[ \"$1 $2\" = '-a 256' ] || exit 2\ncat >/dev/null\necho '"+digest+"  -'\n")

		if _, stderr, err := f.run(t, "PATH="+dir); err != nil {
			t.Fatalf("install failed: %v\n%s", err, stderr)
		}
		if got := mustRead(t, filepath.Join(f.target, "loupe")); got != standInBinary("1.2.3") {
			t.Errorf("installed binary = %q", got)
		}
	})
}

func TestInstallScriptRefusesWithoutHomeOrDir(t *testing.T) {
	f := newInstallFixture(t)
	f.publish(t, "1.2.3", standInBinary("1.2.3"))
	f.markLatest(t, "1.2.3")
	f.mustRefuse(t, "LOUPE_INSTALL_DIR", "HOME=")
}

// A download cut short MUST run nothing, wherever the cut falls.
func TestInstallScriptTruncated(t *testing.T) {
	script := readRepoFile(t, "install.sh")
	lastCall := strings.LastIndex(script, "main ")
	if lastCall < 0 {
		t.Fatal("install.sh has no call to main")
	}
	for _, cut := range []struct {
		name string
		at   int
	}{
		{"before the call", lastCall},
		{"inside a function", len(script) / 2},
		{"after the first line", strings.Index(script, "\n") + 1},
	} {
		t.Run(cut.name, func(t *testing.T) {
			f := newInstallFixture(t)
			f.publish(t, "1.2.3", standInBinary("1.2.3"))
			f.markLatest(t, "1.2.3")
			path := filepath.Join(t.TempDir(), "truncated.sh")
			if err := os.WriteFile(path, []byte(script[:cut.at]), 0o644); err != nil {
				t.Fatal(err)
			}
			// sh MAY exit with a syntax error at the cut. Only a request or a write fails the test.
			_, _, _ = f.runScript(t, path)
			if got := f.requests(t); got != nil {
				t.Errorf("requests = %v, want none", got)
			}
			if got := listDir(t, f.target); got != "(no directory)" {
				t.Errorf("target holds %s", got)
			}
		})
	}
}

func TestInstallScriptPathWarnings(t *testing.T) {
	publish := func(t *testing.T) *installFixture {
		f := newInstallFixture(t)
		f.publish(t, "1.2.3", standInBinary("1.2.3"))
		f.markLatest(t, "1.2.3")
		return f
	}

	t.Run("target not on PATH", func(t *testing.T) {
		f := publish(t)
		_, stderr, err := f.run(t)
		if err != nil {
			t.Fatalf("install failed: %v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "warning: "+f.target+" is not on your PATH") {
			t.Errorf("no PATH warning:\n%s", stderr)
		}
	})

	t.Run("another loupe resolves first", func(t *testing.T) {
		f := publish(t)
		shim := filepath.Join(f.home, "shims")
		if err := os.MkdirAll(shim, 0o755); err != nil {
			t.Fatal(err)
		}
		writeExecutable(t, filepath.Join(shim, "loupe"), standInBinary("0.0.1"))

		_, stderr, err := f.run(t, "PATH="+f.bin+":"+shim+":"+f.target+":/usr/bin:/bin")
		if err != nil {
			t.Fatalf("install failed: %v\n%s", err, stderr)
		}
		if !strings.Contains(stderr, "warning: ") || !strings.Contains(stderr, filepath.Join(shim, "loupe")) {
			t.Errorf("no warning naming the shim:\n%s", stderr)
		}
		if strings.Contains(stderr, "is not on your PATH") {
			t.Errorf("the target is on PATH:\n%s", stderr)
		}
	})

	t.Run("no warning", func(t *testing.T) {
		f := publish(t)
		_, stderr, err := f.run(t, "PATH="+f.bin+":"+f.target+":/usr/bin:/bin")
		if err != nil {
			t.Fatalf("install failed: %v\n%s", err, stderr)
		}
		if stderr != "" {
			t.Errorf("stderr = %q, want nothing", stderr)
		}
	})
}

// goreleaser names the archives, and the script finds them by that name in checksums.txt.
func TestInstallScriptMatchesReleaseArchives(t *testing.T) {
	goreleaser := readRepoFile(t, ".goreleaser.yaml")
	for _, want := range []string{
		"project_name: loupe",
		`name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`,
		"name_template: checksums.txt",
		"- tar.gz",
		"binary: loupe",
	} {
		if !strings.Contains(goreleaser, want) {
			t.Errorf(".goreleaser.yaml no longer carries %q, which install.sh depends on", want)
		}
	}
	if want := `^loupe_${version:-[^_]+}_${os}_${arch}[.]tar[.]gz$`; !strings.Contains(readRepoFile(t, "install.sh"), want) {
		t.Errorf("install.sh no longer matches archives with %q", want)
	}
}

// A person copies this command, so the script's own usage line and both documents MUST name the same URL. The scheme
// is left out because scripts/check-tests.sh refuses a URL literal for any host but loopback and github.com.
func TestInstallScriptURL(t *testing.T) {
	const command = "eriksaulnier.github.io/loupe/install.sh | sh"
	for _, file := range []string{"install.sh", "README.md", "docs/install.md"} {
		if !strings.Contains(readRepoFile(t, file), command) {
			t.Errorf("%s does not carry %q", file, command)
		}
	}
}
