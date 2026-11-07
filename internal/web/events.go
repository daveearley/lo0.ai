package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"lo0.ai/lo0/internal/format"
	"lo0.ai/lo0/internal/scan"
	"lo0.ai/lo0/internal/ui/report"
)

// progressRow is one scan stage as the scanning page lists it.
type progressRow struct {
	Stage string `json:"stage"`
	Label string `json:"label"`
	// State is "pending", "running", "done" or "warn".
	State string `json:"state"`
	Note  string `json:"note"`
}

func progressRows(stages []stage) []progressRow {
	out := make([]progressRow, 0, len(stages))
	for _, st := range stages {
		row := progressRow{Stage: string(st.Stage), Label: report.StageLabel(st.Stage), State: "pending"}
		switch st.Status {
		case scan.StatusRunning:
			row.State = "running"
			if st.Total > 0 {
				row.Note = format.Int(st.Done) + " of " + format.Plural(st.Total, "session", "sessions")
			}
		case scan.StatusDone:
			row.State, row.Note = "done", report.StageFound(st.Stage, st.Found)
			if st.Warnings > 0 {
				row.State, row.Note = "warn", format.Plural(st.Warnings, "warning", "warnings")
			}
		default:
		}
		out = append(out, row)
	}
	return out
}

// events streams scan progress to the scanning page: a "stage" event per
// stage on every change, then "done" once no scan is running, after which
// the page reloads.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")

	changed, stop := s.state.subscribe()
	defer stop()
	for {
		v := s.state.view()
		for _, row := range progressRows(v.stages) {
			data, err := json.Marshal(row)
			if err != nil {
				return
			}
			if _, err := fmt.Fprintf(w, "event: stage\ndata: %s\n\n", data); err != nil {
				return
			}
		}
		if !v.scanning {
			_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
			flusher.Flush()
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-changed:
		}
	}
}
