package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// checkPrompt asks for a reply short enough to be quick on any machine.
const checkPrompt = "Reply with the single word: ready"

// Check returns the step that sends one short request to the served model
// in the protocol the harness speaks, proving the runtime, the model name
// and the API line up before the user starts working.
func Check(s Served, p Protocol) Step {
	var path string
	switch p {
	case Anthropic:
		path = "/v1/messages"
	case OpenAI:
		path = "/v1/chat/completions"
	case Responses:
		path = "/v1/responses"
	}
	return Step{
		Kind:   KindCheck,
		Title:  "Check that " + s.RuntimeName + " answers as " + s.Model,
		Detail: []string{"POST " + s.BaseURL + path},
		Run: func(ctx context.Context, fx Effects, progress func(done, total int64)) error {
			ctx, cancel := context.WithTimeout(ctx, checkTimeout)
			defer cancel()
			if err := ask(ctx, fx.Client, s, p, s.BaseURL+path); err != nil {
				return err
			}
			progress(1, 1)
			return nil
		},
	}
}

func ask(ctx context.Context, client *http.Client, s Served, p Protocol, url string) error {
	request := map[string]any{
		"model":      s.Model,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": checkPrompt}},
	}
	if p == Responses {
		request = responsesRequest(s.Model)
	}
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Token)
	if p == Anthropic {
		req.Header.Set("x-api-key", s.Token)
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s: %s", s.RuntimeName, resp.Status, firstLine(data))
	}
	var reply struct {
		Content []json.RawMessage `json:"content"`
		Choices []json.RawMessage `json:"choices"`
		Output  []json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return fmt.Errorf("decoding the reply: %w", err)
	}
	if len(reply.Content) == 0 && len(reply.Choices) == 0 && len(reply.Output) == 0 {
		return errors.New("the reply had no content")
	}
	return nil
}

// responsesRequest has the shape Codex sends, instructions and a developer
// message before the user's, which some chat templates reject.
func responsesRequest(model string) map[string]any {
	item := func(role, text string) map[string]any {
		return map[string]any{
			"type": "message", "role": role,
			"content": []map[string]string{{"type": "input_text", "text": text}},
		}
	}
	return map[string]any{
		"model":             model,
		"max_output_tokens": 16,
		"instructions":      "You are a coding agent.",
		"input":             []map[string]any{item("developer", "Answer in one word."), item("user", checkPrompt)},
	}
}

// firstLine keeps an error body short enough for one line of output.
func firstLine(b []byte) string {
	s, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
