package diff

import "github.com/eriksaulnier/loupe/internal/markdown"

// Outline maps each Markdown file capture outlined to its headings at the head. A nil Outline is a run captured before
// heading anchors, which has no outline file; a run whose diff changes no Markdown file has an empty one.
type Outline map[string][]markdown.Heading
