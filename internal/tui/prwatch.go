package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/style"
)

// The pull request watch re-reads the live pull request while review is open, so a merge, a close or a moved head
// reaches the human before they spend the session on it (specs/050-pr-state-awareness). Tests shorten both.
var (
	prInterval = 60 * time.Second
	prTimeout  = 10 * time.Second
)

type prTickMsg struct{}

type prResultMsg struct {
	pr  github.PullRequest
	err error
}

// liveState is what the checks have found. A failed check sets err and keeps the rest as the last success left it,
// because a failed fetch returns an empty pull request whose empty head would read as moved.
type liveState struct {
	checked bool
	err     bool
	closed  string
	head    string
}

// startPRCheck starts one check, unless the watch is off, one is already armed, or the review has been sent.
func (m *Model) startPRCheck() tea.Cmd {
	if !m.cfg.WatchPR || m.prArmed || m.view == viewPublished || m.err != nil {
		return nil
	}
	m.prArmed = true
	ctx, client, target := m.watchCtx, m.cfg.GitHub, m.target
	if ctx == nil {
		ctx = context.Background()
	}
	return func() tea.Msg {
		if client == nil {
			return prResultMsg{err: errors.New("no GitHub client")}
		}
		c, err := client()
		if err != nil {
			return prResultMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(ctx, prTimeout)
		defer cancel()
		pr, err := c.PullRequest(ctx, target.Owner, target.Repo, target.Number)
		return prResultMsg{pr: pr, err: err}
	}
}

// applyPR records a check's result and arms the next check one interval after it, so checks never overlap.
func (m *Model) applyPR(msg prResultMsg) tea.Cmd {
	if msg.err != nil {
		m.live.err = true
	} else {
		m.live = liveState{checked: true, closed: publish.ClosedSince(m.target, msg.pr), head: msg.pr.HeadSHA}
	}
	m.prPending = ""
	if tag := m.prTag(); prNotice[tag] != "" && !m.prNoticed[tag] {
		m.prPending = tag
	}
	if m.view == viewPublished {
		m.prArmed = false
		return nil
	}
	return tea.Tick(prInterval, func(time.Time) tea.Msg { return prTickMsg{} })
}

// prTag is the header's tag: the first that holds of merged, closed, a moved head and a failed latest check.
func (m *Model) prTag() string {
	switch {
	case m.live.closed == "merged":
		return "PR merged"
	case m.live.closed == "closed":
		return "PR closed"
	case m.live.checked && m.live.head != m.target.HeadSHA:
		return "head moved"
	case m.live.err:
		return "state unknown"
	}
	return ""
}

// prNotice is the one notice each tag raises in a session; state unknown and no tag raise none.
var prNotice = map[string]string{
	"PR merged":  "pull request merged since capture",
	"PR closed":  "pull request closed since capture",
	"head moved": "pull request head moved since capture",
}

// flushPRNotice shows a waiting notice once nothing the human writes or confirms would move under it, and only in an
// update that set no notice of its own, so it never replaces the answer to the human's last key.
func (m *Model) flushPRNotice(gen int) {
	if m.prPending == "" || m.pollHeld() || m.noticeGen != gen || m.err != nil {
		return
	}
	m.say(style.Warn, prNotice[m.prPending])
	m.prNoticed[m.prPending] = true
	m.prPending = ""
}

// StopWatching cancels a check in flight. The program calls nothing after it quits, so reviewSession calls this once
// the program has ended.
func (m *Model) StopWatching() {
	if m.watchCancel != nil {
		m.watchCancel()
	}
}
