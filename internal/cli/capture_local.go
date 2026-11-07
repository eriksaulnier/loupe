package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/local"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/updatecheck"
)

const (
	maxRunTitle = 120
	maxApplier  = 64
	captureFix  = "pass one of: a pull request URL, --changed [--base <ref>], --files <path>..., or --no-files --title <title>"
)

var (
	localCaptureFlags = []string{"changed", "files", "no-files"}
	localOnlyFlags    = []string{"base", "title", "applier"}
	prOnlyFlags       = []string{"repo", "allow-closed"}
)

// captureForm returns the local form the flags name, or "" for a pull request URL, after the form rules.
func captureForm(cmd *cobra.Command, args []string) (string, error) {
	f := cmd.Flags()
	var forms []string
	for _, name := range localCaptureFlags {
		if f.Changed(name) {
			forms = append(forms, name)
		}
	}
	usage := func(message string) error { return refusal.New(refusal.Usage, message, captureFix) }
	switch {
	case len(args) == 1 && len(forms) > 0:
		return "", usage(fmt.Sprintf("a pull request URL cannot be combined with --%s", forms[0]))
	case len(forms) > 1:
		return "", usage(fmt.Sprintf("--%s cannot be combined with --%s", forms[0], forms[1]))
	case len(args) == 0 && len(forms) == 0:
		return "", usage("capture needs a pull request URL, --changed, --files or --no-files")
	}
	if len(args) == 1 {
		for _, name := range localOnlyFlags {
			if f.Changed(name) {
				return "", usage(fmt.Sprintf("--%s applies only to a local capture", name))
			}
		}
		return "", nil
	}
	for _, name := range prOnlyFlags {
		if f.Changed(name) {
			return "", usage(fmt.Sprintf("--%s applies only to a pull request capture", name))
		}
	}
	form := forms[0]
	if f.Changed("base") && form != local.FormChanged {
		return "", usage(fmt.Sprintf("--base applies only to --changed, not --%s", form))
	}
	if title, _ := f.GetString("title"); form == local.FormNoFiles && strings.TrimSpace(title) == "" {
		return "", refusal.New(refusal.Usage, "--no-files needs --title, since no file names the run", "loupe capture --no-files --title <title>")
	}
	return form, nil
}

// checkRunText refuses a --title or --applier that is not one short line of allowed Markdown.
func checkRunText(flag, value string, limit int, kind markdown.Kind) error {
	if value == "" {
		return nil
	}
	fix := fmt.Sprintf("pass --%s as one line of at most %d characters", flag, limit)
	if n := utf8.RuneCountInString(value); n > limit {
		return refusal.New(refusal.Usage, fmt.Sprintf("--%s is %d characters; the limit is %d", flag, n, limit), fix)
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return refusal.New(refusal.Usage, fmt.Sprintf("--%s holds a control character", flag), fix)
	}
	if err := markdown.Check(value, kind, fix); err != nil {
		if r, ok := refusal.As(err); ok {
			return refusal.New(refusal.Usage, fmt.Sprintf("--%s: %s", flag, r.Message), r.Fix)
		}
		return err
	}
	return nil
}

func runLocalCapture(cmd *cobra.Command, deps Deps, form string) error {
	f := cmd.Flags()
	title, _ := f.GetString("title")
	applier, _ := f.GetString("applier")
	source, _ := f.GetString("source")
	model, _ := f.GetString("model")
	if err := checkRunText("title", title, maxRunTitle, markdown.RunTitle); err != nil {
		return err
	}
	if err := checkRunText("applier", applier, maxApplier, markdown.Applier); err != nil {
		return err
	}
	if err := run.ValidateSource(source); err != nil {
		return err
	}
	if err := run.ValidateModel(model); err != nil {
		return err
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	base, _ := f.GetString("base")
	files, _ := f.GetStringArray("files")
	captured, err := local.Capture(local.Request{Form: form, WorkDir: deps.WorkDir, Home: deps.Getenv("HOME"), Base: base, Files: files})
	if err != nil {
		return err
	}
	target := run.Target{
		Kind: run.KindLocal, Title: title, Applier: applier, Source: source, Model: model,
		CapturedAt: deps.Now().UTC(), DiffSHA256: run.DiffSHA256(captured.Diff), Local: &captured.Local,
	}
	empty := draft.NewEmpty()
	draftJSON, err := json.MarshalIndent(empty, "", "  ")
	if err != nil {
		return fmt.Errorf("encode empty draft: %w", err)
	}
	outlineJSON, err := run.EncodeOutline(captured.Outline)
	if err != nil {
		return err
	}
	dir, ref, err := run.NextLocal(root, "capture", deps.Getenv, target, captured.Diff, append(draftJSON, '\n'),
		map[string][]byte{run.OutlineFile: outlineJSON})
	if err != nil {
		return err
	}
	target.Local.N = ref.Local
	target.Schema = run.TargetSchema
	if err := recordRun(cmd, ref, dir); err != nil {
		return err
	}
	// A local capture makes no network call, so it warns only from what an earlier pull request capture cached.
	var warnings []string
	if current := deps.loupeVersion(); updatecheck.Enabled(current, deps.Getenv) {
		if available := updatecheck.Available(current, updatecheck.Cached(updatecheck.CachePath(root))); available != "" {
			warnings = append(warnings, WarnUpdateAvailable)
			es := deps.errStyle()
			_, _ = fmt.Fprintf(deps.Stderr, "%s %s. Ask the human to run %s\n", es.Emphasis(es.Warn).Render("warning:"), available, updatecheck.Command)
		}
	}
	next := []string{
		fmt.Sprintf("loupe add --run %s --from <file> --json", ref),
		fmt.Sprintf("loupe handoff --run %s --json", ref),
	}
	headings := map[string][]markdown.Heading{}
	for _, e := range captured.Outline {
		headings[e.Path] = e.Headings
	}
	if wantJSON(cmd) {
		payload := map[string]any{"target": target, "headings": headings, "next": next}
		if len(warnings) > 0 {
			payload["warnings"] = warnings
		}
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &empty.Version, payload)
	}
	return printLocalCapture(deps, ref, target, next)
}

func printLocalCapture(deps Deps, ref run.Ref, target run.Target, steps []string) error {
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	title := target.Title
	if title == "" {
		title = "(no title)"
	}
	fmt.Fprintf(&b, "%s %s %s  %s\n", s.Emphasis(s.Good).Render(s.Glyphs.Accepted), s.Emphasis(s.Good).Render("captured"),
		s.Accent.Render(ref.String()), s.TruncRight(oneLine(title), max(20, width-len(ref.String())-14)))
	l := target.Local
	switch l.Capture {
	case local.FormChanged:
		against := "HEAD"
		if l.BaseRef != "" {
			against = l.BaseRef
		}
		fmt.Fprintf(&b, "%s\n", s.Dim.Render(fmt.Sprintf("  changes in %s against %s", oneLine(l.Root), oneLine(against))))
	case local.FormNoFiles:
		fmt.Fprintf(&b, "%s\n", s.Dim.Render("  no files; findings are general"))
	}
	if len(l.Files) > 0 {
		fmt.Fprintf(&b, "\n%s\n", s.Heading("files"))
		for _, file := range l.Files {
			mark := ""
			if file.New {
				mark = "  " + s.Dim.Render("new")
			}
			fmt.Fprintf(&b, "  %s%s\n", oneLine(file.Path), mark)
		}
	}
	if len(l.Skipped) > 0 {
		fmt.Fprintf(&b, "\n%s\n", s.Heading("skipped"))
		for _, k := range l.Skipped {
			fmt.Fprintf(&b, "  %s  %s\n", oneLine(k.Path), s.Dim.Render(k.Reason))
		}
	}
	fmt.Fprintf(&b, "\n%s\n", s.Heading("next"))
	for _, step := range steps {
		fmt.Fprintf(&b, "  %s\n", paintCommand(s, oneLine(step)))
	}
	_, err := io.WriteString(deps.Stdout, b.String())
	return err
}
