package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// resolveRun selects the run from --run or the positional reference, then LOUPE_RUN, then the pull request of the
// working directory's current branch.
func resolveRun(cmd *cobra.Command, deps Deps, positional string) (string, run.Ref, error) {
	selected := positional
	if flag := cmd.Flags().Lookup("run"); flag != nil && flag.Changed {
		if positional != "" {
			return "", run.Ref{}, refusal.New(refusal.Usage,
				"a run reference was given both as an argument and with --run",
				"pass the run reference once: "+cmd.CommandPath()+" --run <ref>")
		}
		selected = flag.Value.String()
	}
	if selected == "" {
		selected = deps.Getenv("LOUPE_RUN")
	}
	var ref run.Ref
	if selected != "" {
		parsed, err := run.ParseRef(selected)
		if err != nil {
			return "", run.Ref{}, err
		}
		ref = parsed
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return "", run.Ref{}, err
	}
	if selected == "" {
		if ref, err = run.ResolveBranch(cmd.Context(), root, deps.WorkDir, deps.GitHub); err != nil {
			return "", run.Ref{}, positionalFix(cmd, err)
		}
	}
	dir, resolved, err := run.ResolveRef(root, ref)
	if err != nil {
		return "", run.Ref{}, positionalFix(cmd, err)
	}
	if err := recordRun(cmd, resolved, dir); err != nil {
		return "", run.Ref{}, err
	}
	return dir, resolved, nil
}

// recordRun names the run for the result envelope. Only the recorded dir is made absolute; every command keeps
// operating on dir as the data root spelled it.
func recordRun(cmd *cobra.Command, ref run.Ref, dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve the run directory %s: %w", dir, err)
	}
	inv := invocationOf(cmd)
	inv.run, inv.dir = ref.String(), abs
	return nil
}

// positionalFix rewrites a fix naming --run for review and publish, which take the run reference as their argument.
func positionalFix(cmd *cobra.Command, err error) error {
	r, ok := refusal.As(err)
	if !ok || cmd.Flags().Lookup("run") != nil {
		return err
	}
	r.Fix = strings.ReplaceAll(r.Fix, "--run <ref>", cmd.CommandPath()+" <ref>")
	return r
}
