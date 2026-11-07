//go:build js && wasm

// Command lo0wasm is lo0's assessment of named hardware for the lo0.ai
// website. It sets globalThis.lo0 with two functions that take and return
// JSON: known() lists the hardware, and assess(request) matches it.
package main

import (
	"context"
	"encoding/json"
	"syscall/js"

	"lo0.ai/lo0/internal/browser"
)

func main() {
	js.Global().Set("lo0", js.ValueOf(map[string]any{
		"known":  js.FuncOf(func(js.Value, []js.Value) any { return encode(browser.Known()) }),
		"assess": js.FuncOf(assess),
	}))
	js.Global().Get("dispatchEvent").Invoke(js.Global().Get("Event").New("lo0ready"))
	select {}
}

func assess(_ js.Value, args []js.Value) any {
	var req browser.Request
	if len(args) == 0 {
		return encode(failure{"no request"})
	}
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return encode(failure{err.Error()})
	}
	a, err := browser.Assess(context.Background(), req)
	if err != nil {
		return encode(failure{err.Error()})
	}
	return encode(a)
}

type failure struct {
	Error string `json:"error"`
}

func encode(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(failure{err.Error()})
	}
	return string(b)
}
