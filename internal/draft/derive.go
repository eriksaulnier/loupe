package draft

const (
	DispositionAccepted  = "accepted"
	DispositionExcluded  = "excluded"
	DispositionWithdrawn = "withdrawn"
	DispositionPending   = "pending"
)

// Readiness holds id lists; counts are their lengths.
type Readiness struct {
	Ready     bool     `json:"ready"`
	Accepted  []string `json:"accepted"`
	Pending   []string `json:"pending"`
	Excluded  []string `json:"excluded"`
	Withdrawn []string `json:"withdrawn"`
	OpenNotes []string `json:"openNotes"`
}

// disposition treats a decision as current only while its findingRev equals the finding's rev, so any edit after a
// decision puts the finding back in front of the human.
func disposition(d *Draft, f Finding) string {
	decision, ok := d.Decisions[f.ID]
	current := ok && decision.FindingRev == f.Rev
	switch {
	case current && decision.Decision == DecisionExcluded:
		return DispositionExcluded
	case current && decision.Decision == DecisionAccepted && f.Included:
		return DispositionAccepted
	case f.Included:
		return DispositionPending
	default:
		return DispositionWithdrawn
	}
}

func Dispositions(d *Draft) map[string]string {
	out := make(map[string]string, len(d.Findings))
	for _, f := range d.Findings {
		out[f.ID] = disposition(d, f)
	}
	return out
}

func ReadinessOf(d *Draft) Readiness {
	r := Readiness{Accepted: []string{}, Pending: []string{}, Excluded: []string{}, Withdrawn: []string{}, OpenNotes: []string{}}
	for _, f := range d.Findings {
		switch disposition(d, f) {
		case DispositionAccepted:
			r.Accepted = append(r.Accepted, f.ID)
		case DispositionExcluded:
			r.Excluded = append(r.Excluded, f.ID)
		case DispositionPending:
			r.Pending = append(r.Pending, f.ID)
		case DispositionWithdrawn:
			r.Withdrawn = append(r.Withdrawn, f.ID)
		}
	}
	for _, n := range d.Notes {
		if n.Status == NoteOpen {
			r.OpenNotes = append(r.OpenNotes, n.ID)
		}
	}
	r.Ready = len(r.Pending) == 0 && len(r.OpenNotes) == 0
	return r
}

func IncludedCount(d *Draft) int {
	n := 0
	for _, f := range d.Findings {
		if f.Included {
			n++
		}
	}
	return n
}

// Awaiting is the handed-back notes the agent still has to answer: in the set, open and without a reply.
func Awaiting(d *Draft, h *HandBack) []string {
	out := []string{}
	for _, id := range h.Notes {
		for _, n := range d.Notes {
			if n.ID == id && n.Status == NoteOpen && !hasReply(d, id) {
				out = append(out, id)
			}
		}
	}
	return out
}

func hasReply(d *Draft, noteID string) bool {
	for _, r := range d.Replies {
		if r.NoteID == noteID {
			return true
		}
	}
	return false
}
