package transcript

import (
	"bytes"
	"encoding/json"
)

// APIError is the error field of a record. Claude Code writes it in two
// shapes: a bare kind such as "rate_limit" on the response it substitutes for
// a failed one, and an object with retry details on api_error system records.
type APIError struct {
	// Kind is the bare form, such as "rate_limit" or "server_error".
	Kind   string
	Status int
	// NetworkDown is true when the retry happened because the connection
	// failed or the machine was offline.
	NetworkDown bool
}

// Error kinds Claude Code writes on substituted responses.
const (
	ErrorRateLimit = "rate_limit"
)

// UnmarshalJSON implements json.Unmarshaler.
func (e *APIError) UnmarshalJSON(data []byte) error {
	*e = APIError{}
	switch {
	case len(data) > 0 && data[0] == '"':
		return json.Unmarshal(data, &e.Kind)
	case len(data) > 0 && data[0] == '{':
		var obj struct {
			Status      int             `json:"status"`
			NetworkDown bool            `json:"isNetworkDown"`
			Connection  json.RawMessage `json:"connection"`
		}
		if err := json.Unmarshal(data, &obj); err != nil {
			return err
		}
		e.Status = obj.Status
		e.NetworkDown = obj.NetworkDown || present(obj.Connection)
	}
	return nil
}

func present(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(raw, []byte("null"))
}

// ToolResult is the structured result Claude Code keeps beside a tool's
// output. Only the edited file and the shape of its change are read: patch
// lines are reduced to their leading + or -, and created files to a line
// count, so no code is decoded.
type ToolResult struct {
	// Type is "create" when a file was written from scratch.
	Type     string `json:"type"`
	FilePath string `json:"filePath"`
	Patch    []Hunk `json:"structuredPatch"`
	// Content is the line count of a created file's content.
	Content LineCount `json:"content"`
}

// UnmarshalJSON implements json.Unmarshaler. Results that are not objects,
// such as error strings, decode to the zero value.
func (r *ToolResult) UnmarshalJSON(data []byte) error {
	*r = ToolResult{}
	if len(data) == 0 || data[0] != '{' {
		return nil
	}
	type plain ToolResult
	return json.Unmarshal(data, (*plain)(r))
}

// Hunk is one section of a unified diff.
type Hunk struct {
	Lines []LineMark `json:"lines"`
}

// LineMark is the first character of a diff line: '+', '-' or ' '.
type LineMark byte

// UnmarshalJSON implements json.Unmarshaler without decoding the line.
func (m *LineMark) UnmarshalJSON(data []byte) error {
	*m = 0
	if len(data) > 1 && data[0] == '"' {
		*m = LineMark(data[1])
	}
	return nil
}

// LineCount is the number of lines in a JSON string, counted without
// decoding it.
type LineCount int

// UnmarshalJSON implements json.Unmarshaler.
func (n *LineCount) UnmarshalJSON(data []byte) error {
	*n = 0
	if len(data) < 2 || data[0] != '"' {
		return nil
	}
	body := data[1 : len(data)-1]
	if len(body) == 0 {
		return nil
	}
	lines, trailing := 1, false
	for i := 0; i < len(body)-1; i++ {
		if body[i] != '\\' {
			continue
		}
		i++
		if body[i] == 'n' {
			lines++
			trailing = i == len(body)-1
		}
	}
	// A final newline ends the last line rather than starting another.
	if trailing {
		lines--
	}
	*n = LineCount(lines)
	return nil
}

// Changed counts the lines a result added and removed.
func (r ToolResult) Changed() (added, removed int) {
	if r.Type == "create" && len(r.Patch) == 0 {
		return int(r.Content), 0
	}
	for _, h := range r.Patch {
		for _, m := range h.Lines {
			switch m {
			case '+':
				added++
			case '-':
				removed++
			}
		}
	}
	return added, removed
}
