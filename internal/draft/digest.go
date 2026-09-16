package draft

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/eriksaulnier/loupe/internal/findingid"
)

// PublishableSet is what the human could still publish: included findings that are not excluded.
func PublishableSet(d *Draft) []Finding {
	var out []Finding
	for _, f := range d.Findings {
		switch disposition(d, f) {
		case DispositionAccepted, DispositionPending:
			out = append(out, f)
		}
	}
	return out
}

// The digest is published in the reconciliation marker, so the encoding is append-only: a new field goes last with
// every key still present, and no field is removed, renamed or reordered.
type digestDoc struct {
	Summary  string          `json:"summary"`
	Findings []digestFinding `json:"findings"`
}

type digestFinding struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Body         string          `json:"body"`
	Location     *digestLocation `json:"location"`
	General      bool            `json:"general"`
	Label        string          `json:"label"`
	Blocking     bool            `json:"blocking"`
	Confidence   string          `json:"confidence"`
	Severity     string          `json:"severity"`
	SuggestedFix string          `json:"suggestedFix"`
	Impact       string          `json:"impact"`
	Verified     string          `json:"verified"`
	References   []string        `json:"references"`
}

type digestLocation struct {
	Path      string `json:"path"`
	Side      string `json:"side"`
	Line      int    `json:"line"`
	StartLine int    `json:"startLine"`
}

// Digest is hex SHA-256 over compact JSON of the summary and the publishable set sorted by id, with every key present
// and no HTML escaping. Accept decisions leave it unchanged, so it can be computed before and after the human decides.
func Digest(d *Draft) string {
	doc := digestDoc{Summary: d.Summary, Findings: []digestFinding{}}
	for _, f := range PublishableSet(d) {
		df := digestFinding{ID: f.ID, Title: f.Title, Body: f.Body, General: f.General, Label: f.Label, Blocking: f.Blocking,
			Confidence: f.Confidence, Severity: f.Severity, SuggestedFix: f.SuggestedFix, Impact: f.Impact, Verified: f.Verified,
			References: append([]string{}, f.References...)}
		if f.Location != nil {
			df.Location = &digestLocation{Path: f.Location.Path, Side: f.Location.Side, Line: f.Location.Line, StartLine: f.Location.StartLine}
		}
		doc.Findings = append(doc.Findings, df)
	}
	slices.SortFunc(doc.Findings, func(a, b digestFinding) int { return findingid.Compare(a.ID, b.ID) })
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		panic(fmt.Sprintf("encode digest document: %v", err))
	}
	sum := sha256.Sum256(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	return hex.EncodeToString(sum[:])
}
