package pane

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
)

type herdrPane struct {
	PaneID string `json:"pane_id"`
	Rect   struct {
		Width *int `json:"width"`
	} `json:"rect"`
}

type Herdr struct {
	path   string
	paneID string
	run    Runner
}

// detectHerdr reads the variables Herdr injects into the panes it manages.
func detectHerdr(getenv func(string) string) (Herdr, bool) {
	paneID := getenv("HERDR_PANE_ID")
	if getenv("HERDR_ENV") != "1" || paneID == "" {
		return Herdr{}, false
	}
	path, ok := lookPath(getenv, "herdr")
	if !ok {
		return Herdr{}, false
	}
	return Herdr{path: path, paneID: paneID, run: execRunner}, true
}

func (Herdr) Name() string { return "herdr" }

// Open probes with pane layout, which opens nothing, so a sandboxed agent can rerun safely when only that step failed.
// The layout also reports the agent pane's width, which the agent's own tty often cannot.
func (h Herdr) Open(ctx context.Context, req Request) (Opened, error) {
	fail := func(step, message string) error { return failed("herdr", step, message, req.Fix) }

	stdout, stderr, err := h.run(ctx, h.path, "pane", "layout", "--pane", h.paneID)
	if err != nil {
		return Opened{}, fail("probe", herdrMessage(stderr, err))
	}
	var layout struct {
		Result struct {
			Layout struct {
				Panes []herdrPane `json:"panes"`
			} `json:"layout"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout, &layout); err != nil {
		return Opened{}, fail("probe", "unreadable result: "+err.Error())
	}
	i := slices.IndexFunc(layout.Result.Layout.Panes, func(p herdrPane) bool { return p.PaneID == h.paneID })
	if i < 0 {
		return Opened{}, fail("probe", "the result does not list pane "+h.paneID)
	}
	width, known := 0, false
	if w := layout.Result.Layout.Panes[i].Rect.Width; w != nil {
		width, known = *w, true
	}
	dir := direction(width, known, req.TTYWidth)

	stdout, stderr, err = h.run(ctx, h.path, "pane", "split", "--pane", h.paneID, "--direction", dir, "--focus")
	if err != nil {
		return Opened{}, fail("split", herdrMessage(stderr, err))
	}
	var split struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout, &split); err != nil {
		return Opened{}, fail("split", "unreadable result: "+err.Error())
	}
	paneID := split.Result.Pane.PaneID
	if paneID == "" {
		return Opened{}, fail("split", "the result has no pane_id")
	}

	if _, stderr, err := h.run(ctx, h.path, "pane", "run", paneID, req.Command); err != nil {
		return Opened{}, fail("run", herdrMessage(stderr, err))
	}
	return Opened{PaneID: paneID, Direction: dir}, nil
}

// herdrMessage prefers the message in Herdr's JSON error object on stderr and falls back to whatever it printed.
func herdrMessage(stderr []byte, err error) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(stderr, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	if message := strings.TrimSpace(string(stderr)); message != "" || err == nil {
		return message
	}
	return err.Error()
}
