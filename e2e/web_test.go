package e2e_test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestWeb starts lo0 web as a user would, checks it admits only requests
// that carry its token and name its own address, then crawls every page and
// asset the pages link to.
func TestWeb(t *testing.T) {
	m := newMachine(t)
	m.startOllama("qwen3:4b")
	m.install("claude")
	addr := startWeb(t, m)

	u, err := url.Parse(addr)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "127.0.0.1" {
		t.Errorf("lo0 web serves on %s, want the loopback address", u.Host)
	}
	token := u.Query().Get("token")
	base := "http://" + u.Host
	bare := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	t.Run("refuses requests without the token", func(t *testing.T) {
		for _, path := range []string{"/", "/models", "/assets/app.css", "/?token=" + strings.Repeat("0", len(token))} {
			resp := get(t, bare, base+path, nil)
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("GET %s answered %d, want 403", path, resp.StatusCode)
			}
			if !strings.Contains(resp.Header.Get("Content-Security-Policy"), "default-src 'none'") {
				t.Errorf("GET %s has no locked-down Content-Security-Policy", path)
			}
		}
	})
	t.Run("refuses another host name", func(t *testing.T) {
		resp := get(t, bare, addr, map[string]string{"Host": "lo0.attacker.example:" + u.Port()})
		if resp.StatusCode != http.StatusMisdirectedRequest {
			t.Errorf("a request naming another host answered %d, want 421", resp.StatusCode)
		}
	})
	t.Run("moves the token into a cookie", func(t *testing.T) {
		resp := get(t, bare, addr, nil)
		if resp.StatusCode != http.StatusSeeOther || strings.Contains(resp.Header.Get("Location"), token) {
			t.Errorf("the printed address answered %d to %q, want a redirect without the token", resp.StatusCode, resp.Header.Get("Location"))
		}
		if len(resp.Cookies()) != 1 || !resp.Cookies()[0].HttpOnly {
			t.Errorf("the printed address set cookies %v, want one HttpOnly cookie", resp.Cookies())
		}
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 30 * time.Second, Jar: jar}
	get(t, client, addr, nil)
	waitForScan(t, client, base)

	t.Run("accepts forms only from its own pages", func(t *testing.T) {
		post := func(origin string) int {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base+"/scan", nil)
			if err != nil {
				t.Fatal(err)
			}
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			resp, err := bare.Do(withCookies(req, jar))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			return resp.StatusCode
		}
		for _, origin := range []string{"", "null", "https://attacker.example"} {
			if code := post(origin); code != http.StatusForbidden {
				t.Errorf("POST /scan from origin %q answered %d, want 403", origin, code)
			}
		}
		if code := post(base); code >= 400 {
			t.Errorf("POST /scan from lo0's own origin answered %d", code)
		}
		waitForScan(t, client, base)
	})

	t.Run("every linked page and asset loads", func(t *testing.T) {
		crawl(t, client, base)
	})
}

// startWeb runs lo0 web until the test ends and returns the address it
// printed, token included.
func startWeb(t *testing.T, m *machine) string {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cmd := m.command(ctx, "web", "--no-open")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stop(cmd.Process)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			cancel()
			t.Errorf("lo0 web did not stop within 15s of being interrupted")
			<-done
		}
		cancel()
	})

	found := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			if line := strings.TrimSpace(s.Text()); strings.HasPrefix(line, "http://") {
				found <- line
			}
		}
		close(found)
	}()
	select {
	case addr, ok := <-found:
		if !ok {
			t.Fatal("lo0 web exited without printing its address")
		}
		return addr
	case <-time.After(runTimeout):
		t.Fatal("lo0 web printed no address")
		return ""
	}
}

// stop interrupts lo0 the way Ctrl-C would. Windows cannot send that to
// another process, so there it is killed.
func stop(p *os.Process) {
	if runtime.GOOS == "windows" || p.Signal(os.Interrupt) != nil {
		_ = p.Kill()
	}
}

// waitForScan waits until the home page shows the assessment rather than
// the scan's progress.
func waitForScan(t *testing.T, client *http.Client, base string) {
	t.Helper()
	deadline := time.Now().Add(runTimeout)
	for time.Now().Before(deadline) {
		if !strings.Contains(get(t, client, base+"/", nil).body, "data-scanning") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("lo0 web was still scanning after " + runTimeout.String())
}

var linkPattern = regexp.MustCompile(`(?:href|src)="(/[^"#]*)"`)

// crawl visits every same-origin page and asset reachable from the home
// page and checks each loads, is private and fetches nothing from another
// host.
func crawl(t *testing.T, client *http.Client, base string) {
	t.Helper()
	seen := map[string]bool{"/": true}
	queue := []string{"/"}
	for len(queue) > 0 && len(seen) < 200 {
		path := queue[0]
		queue = queue[1:]
		resp := get(t, client, base+path, nil)
		page := resp.body
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s answered %d", path, resp.StatusCode)
			continue
		}
		noSecrets(t, page)
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			continue
		}
		for _, external := range []string{`src="http`, `src="//`, `href="//`, `@import url(http`} {
			if strings.Contains(page, external) {
				t.Errorf("%s loads something from another host (%s)", path, external)
			}
		}
		for _, match := range linkPattern.FindAllStringSubmatch(page, -1) {
			link := strings.ReplaceAll(match[1], "&amp;", "&")
			if !seen[link] && link != "/events" {
				seen[link] = true
				queue = append(queue, link)
			}
		}
	}
	pages := slices.Collect(func(yield func(string) bool) {
		for p := range seen {
			if !strings.HasPrefix(p, "/assets/") && !yield(p) {
				return
			}
		}
	})
	t.Logf("crawled %d pages and %d assets", len(pages), len(seen)-len(pages))
	for _, want := range []string{"/", "/models", "/installed", "/claude"} {
		if !slices.Contains(pages, want) {
			t.Errorf("no page links to %s; crawled %v", want, pages)
		}
	}
}

// response is an answer read in full.
type response struct {
	StatusCode int
	Header     http.Header
	cookies    []*http.Cookie
	body       string
}

func (r response) Cookies() []*http.Cookie { return r.cookies }

func get(t *testing.T, client *http.Client, rawURL string, headers map[string]string) response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{StatusCode: resp.StatusCode, Header: resp.Header, cookies: resp.Cookies(), body: string(b)}
}

// withCookies adds the jar's cookies to a request sent by a client without
// the jar.
func withCookies(req *http.Request, jar http.CookieJar) *http.Request {
	for _, c := range jar.Cookies(req.URL) {
		req.AddCookie(c)
	}
	return req
}
