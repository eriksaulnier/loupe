package draft

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ErrNoChange is returned by a mutation fn that left the draft as it was, so Mutate writes nothing and keeps the
// version; a bumped version would refuse the next decision made against the unchanged draft as stale.
var ErrNoChange = errors.New("draft unchanged")

func Load(dir string) (*Draft, error) {
	path := filepath.Join(dir, "draft.json")
	var d Draft
	if err := run.ReadJSON(path, &d, schemaVersion); err != nil {
		return nil, err
	}
	// A draft that decodes with a nil collection would make later mutators panic, so it is refused as damaged.
	problem := ""
	switch {
	case d.Findings == nil:
		problem = "findings is missing or null"
	case d.Decisions == nil:
		problem = "decisions is missing or null"
	case d.Notes == nil:
		problem = "notes is missing or null"
	case d.Replies == nil:
		problem = "replies is missing or null"
	}
	for i, a := range d.Assessments {
		if problem == "" && (a.Ref == "" || (a.Status != StatusOpen && a.Status != StatusAddressed) || a.Finding.ID == "" || a.Finding.Title == "") {
			problem = fmt.Sprintf("assessment %d lacks a ref, a known status, or its finding's id or title", i+1)
		}
	}
	if problem != "" {
		return nil, run.RecordRefusal(path, errors.New(problem))
	}
	// A draft from before schema 6 holds no stamps. Each finding reads as changed at the draft's version, so the
	// per-finding check is as strict as the draft-wide one until a mutation stamps it.
	if d.Schema < 6 {
		d.ChangedIn = map[string]int{}
		for _, f := range d.Findings {
			d.ChangedIn[f.ID] = d.Version
		}
	}
	return &d, nil
}

const versionFix = "re-read with loupe show --json and retry"

// Mutate is the only way a draft changes: read, check the expected version and apply fn all under the run lock, so
// concurrent writers serialize and a failed fn leaves the file untouched.
func Mutate(dir, command string, expectVersion *int, getenv func(string) string, fn func(*Draft) error) (*Draft, error) {
	return mutate(dir, command, func(loaded *Draft) error {
		if expectVersion != nil && *expectVersion != loaded.Version {
			return staleDraft(loaded, *expectVersion)
		}
		return nil
	}, getenv, fn)
}

// MutateFinding is Mutate for a change to one finding: expectVersion is the draft version the caller read the
// finding at, and it refuses only when that finding or its decision changed since. A change to another finding
// passes, so an agent editing one finding does not race the human deciding the rest. A finding that does not exist
// passes the check, so fn reports it.
func MutateFinding(dir, command, findingID string, expectVersion *int, getenv func(string) string, fn func(*Draft) error) (*Draft, error) {
	return mutate(dir, command, func(loaded *Draft) error {
		if expectVersion == nil {
			return nil
		}
		if *expectVersion > loaded.Version {
			return staleDraft(loaded, *expectVersion)
		}
		if at := loaded.ChangedIn[findingID]; at > *expectVersion {
			return refusal.New(refusal.Version, fmt.Sprintf("%s changed at draft version %d, after version %d", findingID, at, *expectVersion), versionFix)
		}
		return nil
	}, getenv, fn)
}

func staleDraft(d *Draft, expected int) error {
	return refusal.New(refusal.Version, fmt.Sprintf("draft is at version %d, expected %d", d.Version, expected), versionFix)
}

func mutate(dir, command string, check func(*Draft) error, getenv func(string) string, fn func(*Draft) error) (*Draft, error) {
	var d *Draft
	err := underLock(dir, command, getenv, func(loaded *Draft) error {
		if err := refuseReturned(dir); err != nil {
			return err
		}
		if err := check(loaded); err != nil {
			return err
		}
		states := findingStates(loaded)
		before := len(loaded.Notes)
		if err := fn(loaded); errors.Is(err, ErrNoChange) {
			d = loaded
			return nil
		} else if err != nil {
			return err
		}
		// A note is handed to the agent as it is written, and before the draft, so no reader sees one without the other.
		// An id whose draft write then fails is harmless: a note awaits only while it exists.
		if len(loaded.Notes) > before {
			if _, err := handBack(dir, loaded, loaded.Notes[before:]); err != nil {
				return err
			}
		}
		loaded.Version++
		stampChanged(loaded, states)
		loaded.Schema = schemaVersion
		if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), loaded); err != nil {
			return err
		}
		d = loaded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// findingState is what a per-finding check guards: the finding as stored, its decision, its notes and the human's
// replies to them. A note the human adds, resolves or dismisses changes what the agent's edit answers, so it moves
// the finding. The agent's own replies do not, so replying before an edit does not refuse it.
type findingState struct {
	Finding      Finding   `json:"finding"`
	Decision     *Decision `json:"decision"`
	Notes        []Note    `json:"notes"`
	HumanReplies []Reply   `json:"humanReplies"`
}

// findingStates encodes each finding's state, so a mutation can tell afterwards which findings it changed. A state
// that does not encode is left empty: it holds only draft values, so the draft write that follows refuses the same
// value and the mutation fails.
func findingStates(d *Draft) map[string]string {
	noteFinding := map[string]string{}
	notes := map[string][]Note{}
	for _, n := range d.Notes {
		noteFinding[n.ID] = n.FindingID
		notes[n.FindingID] = append(notes[n.FindingID], n)
	}
	replies := map[string][]Reply{}
	for _, r := range d.Replies {
		if r.By == ByHuman {
			id := noteFinding[r.NoteID]
			replies[id] = append(replies[id], r)
		}
	}
	out := make(map[string]string, len(d.Findings))
	for _, f := range d.Findings {
		st := findingState{Finding: f, Notes: notes[f.ID], HumanReplies: replies[f.ID]}
		if dec, ok := d.Decisions[f.ID]; ok {
			st.Decision = &dec
		}
		data, _ := json.Marshal(st)
		out[f.ID] = string(data)
	}
	return out
}

// stampChanged records d.Version against every finding whose state differs from before, or that is new.
func stampChanged(d *Draft, before map[string]string) {
	for id, state := range findingStates(d) {
		if was, ok := before[id]; !ok || was != state {
			if d.ChangedIn == nil {
				d.ChangedIn = map[string]int{}
			}
			d.ChangedIn[id] = d.Version
		}
	}
}

// underLock loads the draft under the run lock and holds the lock until fn returns.
func underLock(dir, command string, getenv func(string) string, fn func(*Draft) error) (err error) {
	held, err := run.Lock(dir, command, getenv)
	if err != nil {
		return err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			err = unlockErr
		}
	}()
	d, err := Load(dir)
	if err != nil {
		return err
	}
	return fn(d)
}
