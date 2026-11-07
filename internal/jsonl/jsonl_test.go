package jsonl_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"lo0.ai/lo0/internal/jsonl"
)

type rec struct {
	N int `json:"n"`
}

func TestRecords(t *testing.T) {
	in := `{"n":1}` + "\n\n" + `{"n":` + "\n" + `{"n":2,"pad":"` + strings.Repeat("x", 1<<20) + `"}`
	var got []int
	var decodeErrs []int
	for r, err := range jsonl.Records[rec](iotest.HalfReader(strings.NewReader(in))) {
		if de, ok := errors.AsType[*jsonl.DecodeError](err); ok {
			decodeErrs = append(decodeErrs, de.Line)
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, r.N)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 || len(decodeErrs) != 1 || decodeErrs[0] != 3 {
		t.Errorf("records %v, decode errors on lines %v", got, decodeErrs)
	}
}

func TestRecordsReadError(t *testing.T) {
	boom := errors.New("disk gone")
	var last error
	for _, err := range jsonl.Records[rec](iotest.ErrReader(boom)) {
		last = err
	}
	if !errors.Is(last, boom) {
		t.Errorf("error = %v, want the read error", last)
	}
}

// fill is an endless run of one byte.
type fill byte

func (f fill) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(f)
	}
	return len(p), nil
}

// A line over MaxLine is skipped as a decode error, so one corrupt file
// cannot make a worker hold it all in memory, and reading carries on.
func TestRecordsSkipsLinesOverMaxLine(t *testing.T) {
	huge := func() io.Reader { return io.LimitReader(fill('x'), jsonl.MaxLine+10) }
	for name, in := range map[string]io.Reader{
		"in the middle": io.MultiReader(strings.NewReader(`{"n":1}`+"\n"+`{"pad":"`), huge(), strings.NewReader(`"}`+"\n"+`{"n":2}`+"\n")),
		"at the end":    io.MultiReader(strings.NewReader(`{"n":1}`+"\n"+`{"n":2}`+"\n"+`{"pad":"`), huge()),
	} {
		t.Run(name, func(t *testing.T) {
			var got []int
			var tooLong int
			for r, err := range jsonl.Records[rec](in) {
				switch {
				case errors.Is(err, jsonl.ErrLineTooLong):
					tooLong++
				case err != nil:
					t.Fatal(err)
				default:
					got = append(got, r.N)
				}
			}
			if len(got) != 2 || got[0] != 1 || got[1] != 2 || tooLong != 1 {
				t.Errorf("records %v, %d lines too long; want [1 2] and 1", got, tooLong)
			}
		})
	}
}
