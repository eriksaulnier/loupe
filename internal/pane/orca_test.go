package pane

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const (
	handshake   = "[relay-connect] Handshake OK at version=0.1.0+955da90487c0\n"
	orcaHandle  = "term_agent"
	orcaNew     = "term_28c2bdd2-5b1e"
	orcaShow    = `{"ok":true}`
	orcaSplit   = `{"id":"r1","ok":true,"result":{"split":{"handle":"` + orcaNew + `","tabId":"t1","paneRuntimeId":1,"leafId":"l1"}},"_meta":{"runtimeId":"rt"}}`
	orcaSwitch  = `{"ok":true}`
	orcaStale   = `{"ok":false,"error":{"code":"terminal_handle_stale","message":"terminal handle term_agent is stale"}}`
	orcaStaleIs = "terminal handle term_agent is stale"
	orcaTab     = "tab_agent"
)

// orcaLayout lists two worktrees, the agent's tab in the second, with active the tab each group shows. The first group
// carries a tab entry without a tabId, which the walk MUST step over.
func orcaLayout(active string) string {
	return `{"ok":true,"result":{"terminals":[],"visualLayouts":[` +
		`{"worktreeId":"w0","root":{"type":"group","groupId":"g0","activeTabId":"tab_x","tabs":[{"title":"untitled"},{"tabId":"tab_x","title":"x"}]}},` +
		`{"worktreeId":"w1","root":` + orcaGroup(active) + `}]}}`
}

func orcaGroup(active string) string {
	return `{"type":"group","groupId":"g1","activeTabId":"` + active + `","tabs":[` +
		`{"tabId":"tab_other","title":"other"},{"tabId":"` + orcaTab + `","title":"agent"}]}`
}

// orcaNestedLayout puts the agent's group under a split node rather than at the worktree's root.
func orcaNestedLayout(active string) string {
	return `{"ok":true,"result":{"terminals":[],"visualLayouts":[{"worktreeId":"w1","root":{"type":"split","direction":"vertical",` +
		`"first":{"type":"group","groupId":"g0","activeTabId":"tab_x","tabs":[{"tabId":"tab_x","title":"x"}]},` +
		`"second":` + orcaGroup(active) + `}}]}}`
}

func orcaFor(f *fakeRunner) Orca {
	f.key = 1
	return Orca{path: "/bin/orca", handle: orcaHandle, tabID: orcaTab, run: f.run}
}

func TestDetectOrcaNeedsHandleAndBinary(t *testing.T) {
	bin := t.TempDir()
	orca := filepath.Join(bin, "orca")
	if err := os.WriteFile(orca, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExec := t.TempDir()
	if err := os.WriteFile(filepath.Join(notExec, "orca"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := map[string]string{"ORCA_TERMINAL_HANDLE": orcaHandle, "ORCA_TAB_ID": orcaTab, "PATH": bin}
	var read []string
	recording := func(k string) string {
		read = append(read, k)
		return full[k]
	}

	o, ok := detectOrca(recording)
	if !ok || o.path != orca || o.handle != orcaHandle || o.tabID != orcaTab || o.run == nil {
		t.Fatalf("detectOrca = %+v, %v", o, ok)
	}
	for _, k := range read {
		if k != "ORCA_TERMINAL_HANDLE" && k != "ORCA_TAB_ID" && k != "PATH" {
			t.Errorf("detectOrca read %s; tmux overwrites TERM_PROGRAM, so only the handle, the tab and PATH count", k)
		}
	}
	if o, ok := detectOrca(envWith(full, "ORCA_TAB_ID", "", "")); !ok || o.tabID != "" {
		t.Errorf("without ORCA_TAB_ID: detectOrca = %+v, %v; the tab id only gates focus", o, ok)
	}
	for name, getenv := range map[string]func(string) string{
		"handle unset":        envWith(full, "ORCA_TERMINAL_HANDLE", "", ""),
		"orca not on PATH":    envWith(full, "", "PATH", t.TempDir()),
		"orca not executable": envWith(full, "", "PATH", notExec),
		"PATH unset":          envWith(full, "PATH", "", ""),
	} {
		if _, ok := detectOrca(getenv); ok {
			t.Errorf("%s: detectOrca found Orca", name)
		}
	}
}

func TestOpenSendsShowSplitListAndSwitchWhenTheAgentTabIsActive(t *testing.T) {
	for _, c := range []struct {
		name          string
		tty           func() (int, bool)
		direction     string
		orcaDirection string
	}{
		{"tty 120", width(120), "right", "vertical"},
		{"tty 119", width(119), "down", "horizontal"},
		{"tty unreadable", func() (int, bool) { return 0, false }, "right", "vertical"},
		{"no tty source", nil, "right", "vertical"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"show":   {stdout: orcaShow, stderr: handshake},
				"split":  {stdout: orcaSplit, stderr: handshake},
				"list":   {stdout: orcaLayout(orcaTab), stderr: handshake},
				"switch": {stdout: orcaSwitch, stderr: handshake},
			}}
			opened, err := orcaFor(f).Open(context.Background(), Request{Command: command, TTYWidth: c.tty, Fix: fix})
			if err != nil {
				t.Fatal(err)
			}
			if opened != (Opened{PaneID: orcaNew, Direction: c.direction, Focused: true}) {
				t.Fatalf("opened = %+v", opened)
			}
			want := [][]string{
				{"/bin/orca", "terminal", "show", "--terminal", orcaHandle, "--json"},
				{"/bin/orca", "terminal", "split", "--terminal", orcaHandle, "--direction", c.orcaDirection, "--command", command, "--json"},
				orcaListCall,
				{"/bin/orca", "terminal", "switch", "--terminal", orcaNew, "--json"},
			}
			if !slices.EqualFunc(f.calls, want, slices.Equal) {
				t.Fatalf("calls:\n%q\nwant:\n%q", f.calls, want)
			}
		})
	}
}

var orcaListCall = []string{"/bin/orca", "terminal", "list", "--include-visual-layouts", "--json"}

// switch is the only Orca call that moves the view, so an agent working in a background tab keeps it only if
// switch never runs.
func TestOpenSkipsSwitchWhenTheAgentTabIsInTheBackground(t *testing.T) {
	for name, layout := range map[string]string{"group at root": orcaLayout("tab_other"), "group under a split": orcaNestedLayout("tab_other")} {
		t.Run(name, func(t *testing.T) {
			testOpenSkipsSwitch(t, layout)
		})
	}
}

func testOpenSkipsSwitch(t *testing.T, layout string) {
	t.Helper()
	f := &fakeRunner{replies: map[string]reply{
		"show":   {stdout: orcaShow, stderr: handshake},
		"split":  {stdout: orcaSplit, stderr: handshake},
		"list":   {stdout: layout, stderr: handshake},
		"switch": {stdout: orcaSwitch, stderr: handshake},
	}}
	opened, err := orcaFor(f).Open(context.Background(), Request{Command: command, TTYWidth: width(150), Fix: fix})
	if err != nil {
		t.Fatal(err)
	}
	if opened != (Opened{PaneID: orcaNew, Direction: "right", Focused: false}) {
		t.Fatalf("opened = %+v", opened)
	}
	if len(f.calls) != 3 || !slices.Equal(f.calls[2], orcaListCall) {
		t.Fatalf("calls = %q, want show, split and list with no switch", f.calls)
	}
}

func TestOpenSwitchesWithoutListingWhenTheTabIsUnknown(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"show":   {stdout: orcaShow, stderr: handshake},
		"split":  {stdout: orcaSplit, stderr: handshake},
		"switch": {stdout: orcaSwitch, stderr: handshake},
	}}
	o := orcaFor(f)
	o.tabID = ""
	opened, err := o.Open(context.Background(), Request{Command: command, TTYWidth: width(150), Fix: fix})
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Focused || len(f.calls) != 3 || f.calls[2][2] != "switch" {
		t.Fatalf("opened = %+v, calls = %q, want show, split and switch", opened, f.calls)
	}
}

func TestOpenRefusesAtLayout(t *testing.T) {
	for _, c := range []struct {
		name    string
		list    reply
		message string
	}{
		{"tab in no group", reply{stdout: strings.ReplaceAll(orcaLayout("tab_other"), orcaTab, "tab_gone"), stderr: handshake},
			"orca layout: no tab group holds tab " + orcaTab + "; a pane may already be open"},
		{"group without activeTabId", reply{stdout: strings.Replace(orcaLayout(orcaTab), `"activeTabId":"`+orcaTab+`",`, "", 1), stderr: handshake},
			"orca layout: the group holding tab " + orcaTab + " has no activeTabId; a pane may already be open"},
		{"unreadable listing", reply{stdout: "nope", stderr: handshake}, ""},
		{"listing fails", reply{stderr: handshake + "error: relay refused\n", err: errExit1},
			"orca layout: error: relay refused; a pane may already be open"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"show":   {stdout: orcaShow, stderr: handshake},
				"split":  {stdout: orcaSplit, stderr: handshake},
				"list":   c.list,
				"switch": {stdout: orcaSwitch, stderr: handshake},
			}}
			_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
			if c.message != "" {
				assertOrcaRefusal(t, err, "layout", c.message)
			} else if r, ok := refusal.As(err); !ok || r.Details["step"] != "layout" ||
				!strings.HasPrefix(r.Message, "orca layout: unreadable result: ") {
				t.Fatalf("err = %#v, want pane-failed at layout with an unreadable result", err)
			}
			if len(f.calls) != 3 {
				t.Fatalf("made %d calls, want 3 with no switch: %q", len(f.calls), f.calls)
			}
		})
	}
}

func assertOrcaRefusal(t *testing.T, err error, step, message string) {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.PaneFailed || r.Fix != fix || r.Message != message ||
		r.Details["host"] != "orca" || r.Details["step"] != step {
		t.Fatalf("err = %#v, want pane-failed at %s with message %q and the fix", err, step, message)
	}
}

func TestOpenRefusesOnAStaleHandle(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"show": {stdout: orcaStale, stderr: handshake, err: errExit1}}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertOrcaRefusal(t, err, "probe", "orca probe: "+orcaStaleIs)
	if len(f.calls) != 1 {
		t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
	}
}

func TestOpenRefusesOnASplitWithoutAHandle(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"show":   {stdout: orcaShow, stderr: handshake},
		"split":  {stdout: `{"ok":true,"result":{"split":{}}}`, stderr: handshake},
		"switch": {stdout: orcaSwitch, stderr: handshake},
	}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertOrcaRefusal(t, err, "split", "orca split: the result has no handle; a pane may already be open")
	if len(f.calls) != 2 {
		t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
	}
}

func TestOpenRefusesOnASwitchFailureAfterASplit(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"show":   {stdout: orcaShow, stderr: handshake},
		"split":  {stdout: orcaSplit, stderr: handshake},
		"list":   {stdout: orcaLayout(orcaTab), stderr: handshake},
		"switch": {stderr: handshake + "error: no such terminal\n", err: errExit1},
	}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertOrcaRefusal(t, err, "switch", "orca switch: error: no such terminal; a pane may already be open")
	if len(f.calls) != 4 {
		t.Fatalf("made %d calls, want 4: %q", len(f.calls), f.calls)
	}
}

// Orca's stdout carries the success envelope, so output that is not JSON refuses at the step that produced it.
func TestOpenRefusesOnUnreadableOutput(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"show": {stdout: "nope", stderr: handshake}}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	r, ok := refusal.As(err)
	if !ok || r.Details["step"] != "probe" || !strings.HasPrefix(r.Message, "orca probe: unreadable result: ") {
		t.Fatalf("err = %#v, want pane-failed at probe with an unreadable result", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
	}
}

func TestOpenRefusesWhenOrcaSaysNotOK(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"show": {stdout: `{"ok":false}`, stderr: handshake}}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertOrcaRefusal(t, err, "probe", "orca probe: the result is not ok")
}

// Orca can exit 0 and still report a failure in its envelope; the refusal carries that message, not a generic one.
func TestOpenRefusesWithTheMessageOfANotOKReplyOnExitZero(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"show":  {stdout: orcaShow, stderr: handshake},
		"split": {stdout: `{"ok":false,"error":{"message":"split refused: tab is closing"}}`, stderr: handshake},
	}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertOrcaRefusal(t, err, "split", "orca split: split refused: tab is closing; a pane may already be open")
	if len(f.calls) != 2 {
		t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
	}
}

func TestOrcaMessagePrefersStdoutErrorThenNonHandshakeStderr(t *testing.T) {
	for _, c := range []struct {
		name, stdout, stderr string
		err                  error
		want                 string
	}{
		{"stdout error object", orcaStale, handshake, errExit1, orcaStaleIs},
		{"stderr past the handshake", "", handshake + "error: relay refused\n\n", errExit1, "error: relay refused"},
		{"only the handshake", "", handshake, errExit1, "exit status 1"},
		{"nothing printed", "", "", errExit1, "exit status 1"},
	} {
		if got := orcaMessage([]byte(c.stdout), []byte(c.stderr), c.err); got != c.want {
			t.Errorf("%s: orcaMessage = %q, want %q", c.name, got, c.want)
		}
	}
}
