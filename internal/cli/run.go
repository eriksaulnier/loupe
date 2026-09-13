package cli

import (
	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// resolveRun selects the run from --run or the positional reference, then LOUPE_RUN.
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
	if selected == "" {
		return "", run.Ref{}, refusal.New(refusal.NoRun, "no run selected",
			"loupe capture <pr-url>, or pass --run owner/repo#123")
	}
	ref, err := run.ParseRef(selected)
	if err != nil {
		return "", run.Ref{}, err
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return "", run.Ref{}, err
	}
	dir, resolved, err := run.ResolveRef(root, ref)
	if err != nil {
		return "", run.Ref{}, err
	}
	invocationOf(cmd).run = resolved.String()
	return dir, resolved, nil
}
