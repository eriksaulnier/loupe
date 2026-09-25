package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/style"
)

// Help is the one surface Constitution I requires the whole workflow to be completable from, so every sentence of it
// is fixed; only its structure and color are decided here.

// stepTagColumn is where the human-only tag sits in the workflow, past the longest step a human runs.
const stepTagColumn = 46

const (
	groupAgent  = "agent"
	groupAnyone = "anyone"
	groupHuman  = "human"
)

const (
	rootTagline = "Collect pull request review findings for a human to decide and publish."
	rootLead    = "An agent files findings into a local draft; the human decides each finding and posts exactly\none confirmed GitHub review."
	rootFlow    = "capture → add → summary → review → publish"
	rootRule    = "review and publish are human-only. An agent MUST NOT operate them, pipe confirmation into\nthem, drive them through a pseudo-terminal, or start publish by any route. An agent MAY run\nloupe handoff to start review in a new terminal pane the human sees, and MUST NOT then send\nto, read, resize, close or reuse that pane; otherwise it tells the human to run loupe review.\nAn agent MAY block on loupe wait for the human's notes and MUST pass --run to it."
	sendBackWhy = "when the human sends findings back from review with notes"
	runSelect   = "Run selection, in order: --run <ref> (or the <ref> argument of handoff, review and publish),\nthen LOUPE_RUN, then the pull request of the current branch in the working directory at its\nnewest round."
)

type workflowStep struct {
	command string
	what    string
	human   bool
}

var workflowSteps = []workflowStep{
	{"loupe capture https://github.com/owner/repo/pull/123 --json", "capture the pull request into a new round; prints the run reference", false},
	{"loupe add --run owner/repo#123 --from findings.json --json", "file findings, one object or an array", false},
	{"loupe summary --run owner/repo#123 --from summary.json --expect-findings 2 --json", "set the summary and confirm how many findings landed", false},
	{"loupe handoff --run owner/repo#123 --json", "open review for the human in a new pane, where the terminal can", false},
	{"loupe wait --run owner/repo#123 --json", "block until the human sends a finding back or publishes", false},
	{"loupe review owner/repo#123", "the human decides each finding with the diff in view", true},
	{"loupe publish owner/repo#123 --action comment", "the human confirms and posts one review", true},
}

var sendBackLoop = [][2]string{
	{"loupe feedback --run owner/repo#123 --json", "read notes, dispositions and readiness"},
	{"loupe edit f-001 --run owner/repo#123 --from f.json", "change the finding; clears its decision"},
	{`loupe reply n-001 --run owner/repo#123 --body "Fixed."`, "answer the note"},
}

var runReferences = [][2]string{
	{"owner/repo#123", "the newest round of pull request 123"},
	{"owner/repo#123@2", "round 2"},
}

var conventions = [][2]string{
	{"--json", "print exactly one JSON result object on stdout; diagnostics go to stderr"},
	{"--from <file>|-", "read JSON input from a file, or from stdin with -"},
	{"--expect-version <n>", "refuse unless the draft is at version n (add, edit, summary, assess, reply)"},
	{"--by agent|human", "who makes the change (default agent)"},
	{"--version", "print loupe's own version"},
}

const conventionsTail = "Exit 0 on success, 1 on refusal, 2 on usage error. Every refusal names a code and a fix.\nEvery command's --help shows its JSON input and result shapes."

var environment = [][2]string{
	{"LOUPE_HOME", "data root (default $XDG_DATA_HOME/loupe, else ~/.local/share/loupe)"},
	{"LOUPE_RUN", "default run reference"},
	{"LOUPE_LOCK_TIMEOUT_MS", "how long to wait for the run lock (default 3000, max 60000)"},
	{"NO_COLOR, TERM, LANG/LC_ALL", "honored for color, plain-mode fallback and glyph selection"},
	{"LOUPE_ICONS", "ascii, unicode (the default) or nerd, which needs a Nerd Font"},
}

// setHelp replaces cobra's help and usage output for the whole command tree, so the commands are listed once, under
// their own heading, instead of once as Available Commands and again in the usage tail.
func setHelp(root *cobra.Command, deps Deps) {
	root.AddGroup(
		&cobra.Group{ID: groupAgent, Title: groupAgent},
		&cobra.Group{ID: groupAnyone, Title: groupAnyone},
		&cobra.Group{ID: groupHuman, Title: groupHuman},
	)
	root.SetHelpFunc(func(c *cobra.Command, _ []string) {
		s := deps.outStyle()
		text := commandHelp(s, c)
		if !c.HasParent() {
			text = rootHelpText(s, c)
		}
		_, _ = fmt.Fprintln(c.OutOrStdout(), strings.TrimRight(text, "\n"))
	})
	root.SetUsageFunc(func(c *cobra.Command) error {
		_, err := fmt.Fprintf(c.OutOrStdout(), "%s\n  %s\n", deps.outStyle().Heading("usage"), c.UseLine())
		return err
	})
}

func rootHelpText(s style.Style, root *cobra.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n\n%s\n\n", s.Brand(), rootTagline, rootLead)

	fmt.Fprintf(&b, "%s  %s\n", s.Heading("workflow"), rootFlow)
	for i, step := range workflowSteps {
		line := paintCommand(s, step.command)
		if step.human {
			line = style.Pad(line, stepTagColumn) + "  " + s.Accent.Render(groupHuman)
		}
		fmt.Fprintf(&b, "  %s  %s\n     %s\n", s.Dim.Render(fmt.Sprint(i+1)), line, s.Dim.Render(step.what))
	}
	fmt.Fprintf(&b, "\n%s\n\n", indent(paint(s.Bad, rootRule), "  "))

	fmt.Fprintf(&b, "%s\n", s.Heading("commands"))
	for _, group := range []string{groupAgent, groupAnyone, groupHuman} {
		label := s.Dim.Render(group)
		if group == groupHuman {
			label = s.Accent.Render(group)
		}
		for i, c := range commandsInGroup(root, group) {
			lead := style.Pad(label, 6)
			if i > 0 {
				lead = strings.Repeat(" ", 6)
			}
			fmt.Fprintf(&b, "  %s  %s  %s\n", lead, style.Pad(s.Bold.Render(c.Name()), 8), c.Short)
		}
	}

	fmt.Fprintf(&b, "\n%s  %s\n", s.Heading("send-back loop"), s.Dim.Render(sendBackWhy))
	column := 0
	for _, l := range sendBackLoop {
		column = max(column, style.Width(l[0]))
	}
	for _, l := range sendBackLoop {
		fmt.Fprintf(&b, "  %s  %s\n", style.Pad(paintCommand(s, l[0]), column), s.Dim.Render(l[1]))
	}

	fmt.Fprintf(&b, "\n%s\n%s\n%s\n", s.Heading("run references"), table(s, runReferences, 18), indent(runSelect, "  "))
	fmt.Fprintf(&b, "\n%s\n%s\n%s\n", s.Heading("conventions"), table(s, conventions, 22), indent(conventionsTail, "  "))
	fmt.Fprintf(&b, "\n%s\n%s\n", s.Heading("environment"), table(s, environment, 28))
	return b.String()
}

// paintCommand paints a `loupe …` line: the program and the subcommand carry the weight, the arguments do not.
func paintCommand(s style.Style, line string) string {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 || parts[0] != "loupe" {
		return line
	}
	painted := s.Bold.Render(parts[0] + " " + parts[1])
	if len(parts) == 3 {
		painted += " " + parts[2]
	}
	return painted
}

// commandsInGroup lists a group's commands in the order they were added; a command cobra adds itself, help or
// completion, has no group and is not part of the workflow.
func commandsInGroup(root *cobra.Command, group string) []*cobra.Command {
	var out []*cobra.Command
	for _, c := range root.Commands() {
		if c.GroupID == group {
			out = append(out, c)
		}
	}
	return out
}

func table(s style.Style, rows [][2]string, width int) string {
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "  %s  %s\n", style.Pad(s.Accent.Render(r[0]), width), r[1])
	}
	return strings.TrimRight(b.String(), "\n")
}

// paint colors a paragraph line by line; one Render of the whole block would pad every line to the widest of them.
func paint(st lipgloss.Style, text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = st.Render(l)
	}
	return strings.Join(lines, "\n")
}

func indent(text, prefix string) string {
	return prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)
}

func commandHelp(s style.Style, c *cobra.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n  %s\n\n", s.Heading("usage"), c.UseLine())
	fmt.Fprintf(&b, "%s\n", longHelp(s, c.Long))
	if flags := c.NonInheritedFlags().FlagUsages() + c.InheritedFlags().FlagUsages(); strings.TrimSpace(flags) != "" {
		fmt.Fprintf(&b, "\n%s\n%s\n", s.Heading("flags"), strings.TrimRight(flags, "\n"))
	}
	if c.Example != "" {
		fmt.Fprintf(&b, "\n%s\n", s.Heading("examples"))
		for _, line := range strings.Split(strings.TrimRight(c.Example, "\n"), "\n") {
			fmt.Fprintf(&b, "  %s\n", paintCommand(s, strings.TrimSpace(line)))
		}
	}
	return b.String()
}

// longHelp adds headings and column alignment to a command's Long; the words stay the command's own.
func longHelp(s style.Style, long string) string {
	var b strings.Builder
	inTable := false
	for _, line := range strings.Split(strings.TrimRight(long, "\n"), "\n") {
		label, detail, rest := headingOf(line)
		switch {
		case label != "":
			inTable = false
			if detail != "" {
				detail = " " + detail
			}
			if rest != "" {
				rest = "  " + s.Dim.Render(rest)
			}
			fmt.Fprintf(&b, "%s%s%s\n", s.Heading(label), detail, rest)
		case strings.HasSuffix(line, "in this order:"):
			fmt.Fprintf(&b, "%s\n\n%s\n", line, s.Heading("refuses"))
			inTable = true
		case inTable && strings.HasPrefix(line, "  "):
			code, rest, ok := strings.Cut(strings.TrimSpace(line), "  ")
			if !ok {
				inTable = false
				fmt.Fprintln(&b, line)
				break
			}
			fmt.Fprintf(&b, "  %s  %s\n", style.Pad(s.Accent.Render(code), 11), strings.TrimSpace(rest))
		default:
			inTable = inTable && strings.TrimSpace(line) == ""
			fmt.Fprintln(&b, line)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// headingOf splits "Result (--json), alone on stdout …:" into the heading label, the flag parenthetical and the rest.
// A sentence that merely ends in a colon yields no label, so its words are printed as they were written.
func headingOf(line string) (label, detail, rest string) {
	if !strings.HasSuffix(line, ":") || line != strings.TrimLeft(line, " ") {
		return "", "", ""
	}
	body := strings.TrimSuffix(line, ":")
	label = body
	if at := strings.Index(label, " ("); at >= 0 {
		label = label[:at]
	}
	if label == "" || len(strings.Fields(label)) > 3 || len(label) > 24 || strings.ContainsAny(label, ".,;") {
		return "", "", ""
	}
	if strings.ToLower(label[:1]) == label[:1] {
		return "", "", ""
	}
	remainder := strings.TrimPrefix(body, label)
	if strings.HasPrefix(remainder, " (") {
		if end := strings.Index(remainder, ")"); end >= 0 {
			detail, remainder = remainder[1:end+1], remainder[end+1:]
		}
	}
	return label, detail, strings.Trim(remainder, " ,")
}

// version is the build's version, set with -ldflags "-X github.com/eriksaulnier/loupe/internal/cli.version=…"; a build
// that sets nothing says so rather than claiming a release.
var version = "dev"
