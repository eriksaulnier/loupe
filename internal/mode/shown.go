package mode

import (
	"encoding/json"
	"fmt"
)

// Shown is findings as the agent reads them on a run of m's kind. A pull request run shows them as they are. A local
// run stores blocking false and side RIGHT on every finding, which mean nothing to the agent applying them, so its
// findings are re-encoded without those two keys.
func Shown[T any](m Mode, findings []T) (any, error) {
	if _, ok := m.(apply); !ok {
		return findings, nil
	}
	data, err := json.Marshal(findings)
	if err != nil {
		return nil, fmt.Errorf("encode findings: %w", err)
	}
	var out []map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode findings: %w", err)
	}
	for _, f := range out {
		delete(f, "blocking")
		if loc, ok := f["location"].(map[string]any); ok {
			delete(loc, "side")
		}
	}
	return out, nil
}
