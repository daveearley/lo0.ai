package rollout

import (
	"bytes"
	"encoding/json"
)

// Patch is an apply_patch call's input reduced to the files it touches and
// the lines it adds and removes. Inputs that are not patches, such as code
// for the exec tool, decode to the zero value.
type Patch struct {
	// Files are the paths the patch adds, updates or deletes. Paths stay in
	// memory and never reach the profile.
	Files          []string
	Added, Removed int
}

// patchStart begins every apply_patch input.
var patchStart = []byte(`"*** Begin Patch`)

// fileHeaders introduce each file in a patch.
var fileHeaders = [][]byte{[]byte("*** Add File: "), []byte("*** Update File: "), []byte("*** Delete File: ")}

// UnmarshalJSON implements json.Unmarshaler.
func (p *Patch) UnmarshalJSON(data []byte) error {
	*p = Patch{}
	if !bytes.HasPrefix(data, patchStart) {
		return nil
	}
	scanLines(data, func(line []byte) {
		for _, h := range fileHeaders {
			if rest, ok := bytes.CutPrefix(line, h); ok {
				var path string
				if json.Unmarshal(append(append([]byte{'"'}, rest...), '"'), &path) == nil && path != "" {
					p.Files = append(p.Files, path)
				}
				return
			}
		}
		switch {
		case bytes.HasPrefix(line, []byte("***")):
		case bytes.HasPrefix(line, []byte("+")):
			p.Added++
		case bytes.HasPrefix(line, []byte("-")):
			p.Removed++
		}
	})
	return nil
}

// scanLines calls fn with each line of a JSON string, still escaped, without
// decoding it. Anything that is not a string is ignored.
func scanLines(data []byte, fn func(line []byte)) {
	if len(data) < 2 || data[0] != '"' {
		return
	}
	body := data[1 : len(data)-1]
	start := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' || i+1 >= len(body) {
			continue
		}
		if body[i+1] == 'n' {
			fn(body[start:i])
			start = i + 2
		}
		i++
	}
	if start < len(body) {
		fn(body[start:])
	}
}
