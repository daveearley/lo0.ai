package transcript

import (
	"io"
	"iter"

	"lo0.ai/lo0/internal/jsonl"
)

// DecodeError reports a line that is not a valid record.
type DecodeError = jsonl.DecodeError

// Records yields each record in r. A line that fails to decode yields a
// *DecodeError and iteration continues. A read error is yielded once and ends
// iteration. Blank lines are skipped.
func Records(r io.Reader) iter.Seq2[Record, error] {
	return jsonl.Records[Record](r)
}
