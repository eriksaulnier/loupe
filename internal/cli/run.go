package cli

import (
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
			return "", run.Ref{}, err
		}
	}
	dir, resolved, err := run.ResolveRef(root, ref)
	if err != nil {
		return "", run.Ref{}, err
	}
	invocationOf(cmd).run = resolved.String()
	return dir, resolved, nil
}
