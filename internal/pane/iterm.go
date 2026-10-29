package pane

import (
	"context"
	"strconv"
	"strings"
)

// itermFind is shared by every script. It raises an error osascript prints on stderr when no session has the id, which
// is how a probe from a stale ITERM_SESSION_ID fails.
const itermFind = `on findSession(sid)
	tell application "iTerm2"
		repeat with w in windows
			repeat with t in tabs of w
				repeat with s in sessions of t
					if (unique id of s) is sid then return s
				end repeat
			end repeat
		end repeat
	end tell
	error "no iTerm2 session has id " & sid
end findSession
`

const itermProbe = itermFind + `on run argv
	set agent to findSession(item 1 of argv)
	tell application "iTerm2" to return columns of agent
end run`

// iTerm2 names the divider: a vertical split puts the new session to the right.
const itermSplit = itermFind + `on run argv
	set agent to findSession(item 1 of argv)
	tell application "iTerm2"
		tell agent
			if (item 2 of argv) is "right" then
				set opened to (split vertically with default profile)
			else
				set opened to (split horizontally with default profile)
			end if
		end tell
		return unique id of opened
	end tell
end run`

// The split takes focus in its tab, so the script gives it back to the agent's session. It selects no window or tab and
// never activates iTerm2, so neither the view nor the frontmost application moves.
const itermRun = itermFind + `on run argv
	set agent to findSession(item 1 of argv)
	set opened to findSession(item 2 of argv)
	set reviewText to item 3 of argv
	tell application "iTerm2"
		tell opened to write text reviewText
		tell agent to select
	end tell
end run`

type ITerm struct {
	path      string
	sessionID string
	run       Runner
}

// detectITerm needs TERM_PROGRAM because a program started from an iTerm2 shell, such as an editor's terminal, inherits
// ITERM_SESSION_ID but sets its own TERM_PROGRAM. iTerm2 comes last, so the check costs nothing inside an earlier host.
func detectITerm(getenv func(string) string, goos string) (ITerm, bool) {
	if goos != "darwin" || getenv("TERM_PROGRAM") != "iTerm.app" {
		return ITerm{}, false
	}
	id := getenv("ITERM_SESSION_ID")
	i := strings.LastIndex(id, ":")
	if i < 0 || i == len(id)-1 {
		return ITerm{}, false
	}
	path, ok := lookPath(getenv, "osascript")
	if !ok {
		return ITerm{}, false
	}
	return ITerm{path: path, sessionID: id[i+1:], run: execRunner}, true
}

func (ITerm) Name() string { return "iterm2" }

// Open passes every value as a script argument, never spliced into AppleScript source, so a quote in a data root cannot
// change what the script does. Each split starts the profile's shell and the command is typed into it: a command given
// to the split replaces the shell, so the pane would close on a refusal before the human could read it.
func (it ITerm) Open(ctx context.Context, req Request) (Opened, error) {
	call := func(step, script string, values ...string) (string, error) {
		var args []string
		for line := range strings.SplitSeq(script, "\n") {
			args = append(args, "-e", line)
		}
		stdout, stderr, err := it.run(ctx, it.path, append(args, values...)...)
		if err != nil {
			return "", failed("iterm2", step, stderrMessage(stderr, err), req.Fix)
		}
		return strings.TrimSpace(string(stdout)), nil
	}

	columns, err := call("probe", itermProbe, it.sessionID)
	if err != nil {
		return Opened{}, err
	}
	width, _ := strconv.Atoi(columns)
	dir := direction(width, width > 0, req.TTYWidth)

	sessionID, err := call("split", itermSplit, it.sessionID, dir)
	if err != nil {
		return Opened{}, err
	}
	if sessionID == "" {
		return Opened{}, failed("iterm2", "split", "the reply has no session id", req.Fix)
	}
	if _, err := call("run", itermRun, it.sessionID, sessionID, req.Command); err != nil {
		return Opened{}, err
	}
	return Opened{PaneID: sessionID, Direction: dir}, nil
}
