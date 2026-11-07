package cli_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"lo0.ai/lo0/internal/cli"
)

func TestWebServesUntilCancelled(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	ctx, cancel := context.WithCancel(t.Context())
	var opened string
	var status int
	h.app.Browser = func(ctx context.Context, url string) error {
		opened = url
		// Opening the address is what a browser would do first: it must
		// answer, then the test stops the server.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		status = resp.StatusCode
		_ = resp.Body.Close()
		cancel()
		return nil
	}
	if code := h.app.Run(ctx, []string{"web"}); code != cli.ExitOK && code != cli.ExitInterrupted {
		t.Fatalf("exit = %d: %s", code, h.err)
	}
	if !strings.HasPrefix(opened, "http://127.0.0.1:") || !strings.Contains(opened, "/?token=") {
		t.Errorf("opened %q, want a loopback address with a token", opened)
	}
	if !strings.Contains(h.out.String(), opened) || !strings.Contains(h.out.String(), "Only this machine can reach it") {
		t.Errorf("stdout = %q", h.out)
	}
	if status != http.StatusSeeOther {
		t.Errorf("the printed address answered %d, want a redirect that keeps the token in a cookie", status)
	}
}

func TestWebRejectsABadPort(t *testing.T) {
	h := newHarness(fakeService{res: result()})
	if code := h.app.Run(t.Context(), []string{"web", "--port", "70000"}); code != cli.ExitUsage {
		t.Errorf("exit = %d, want usage", code)
	}
	if !strings.Contains(h.err.String(), "--port must be between 0 and 65535") {
		t.Errorf("stderr = %q, want the reason", h.err)
	}
}
