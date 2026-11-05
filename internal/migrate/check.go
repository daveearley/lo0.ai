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
	path := "/v1/messages"
	if p == OpenAI {
		path = "/v1/chat/completions"
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
	body, err := json.Marshal(map[string]any{
		"model":      s.Model,
		"max_tokens": 16,
		"messages":   []map[string]string{{"role": "user", "content": checkPrompt}},
	})
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
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		return fmt.Errorf("decoding the reply: %w", err)
	}
	if len(reply.Content) == 0 && len(reply.Choices) == 0 {
		return errors.New("the reply had no content")
	}
	return nil
}

// firstLine keeps an error body short enough for one line of output.
func firstLine(b []byte) string {
	s, _, _ := strings.Cut(strings.TrimSpace(string(b)), "\n")
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}
