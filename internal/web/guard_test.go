package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testClock() time.Time { return testNow }

func TestGuard(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	h := guard(testAddr, testToken, ok)
	good := &http.Cookie{Name: "lo0-7410", Value: testToken}

	for _, tc := range []struct {
		name     string
		method   string
		target   string
		host     string
		origin   string
		cookie   *http.Cookie
		want     int
		location string
	}{
		{name: "no token", target: "/", want: http.StatusForbidden},
		{name: "wrong token in the address", target: "/?token=nope", want: http.StatusForbidden},
		{name: "token moves to a cookie", target: "/models?show=all&token=" + testToken, want: http.StatusSeeOther, location: "/models?show=all"},
		{name: "cookie", target: "/", cookie: good, want: http.StatusTeapot},
		{name: "wrong cookie", target: "/", cookie: &http.Cookie{Name: "lo0-7410", Value: "nope"}, want: http.StatusForbidden},
		{name: "another copy's cookie", target: "/", cookie: &http.Cookie{Name: "lo0-7411", Value: testToken}, want: http.StatusForbidden},
		{name: "localhost names this machine", target: "/", host: "localhost:7410", cookie: good, want: http.StatusTeapot},
		{name: "another name for this address", target: "/", host: "evil.example:7410", cookie: good, want: http.StatusMisdirectedRequest},
		{name: "another port", target: "/", host: "127.0.0.1:80", cookie: good, want: http.StatusMisdirectedRequest},
		{name: "form from lo0", method: http.MethodPost, target: "/scan", origin: "http://" + testAddr, cookie: good, want: http.StatusTeapot},
		{name: "form from another site", method: http.MethodPost, target: "/scan", origin: "http://evil.example", cookie: good, want: http.StatusForbidden},
		{name: "form with no origin", method: http.MethodPost, target: "/scan", cookie: good, want: http.StatusForbidden},
		{name: "form with a null origin", method: http.MethodPost, target: "/scan", origin: "null", cookie: good, want: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := tc.method
			if method == "" {
				method = http.MethodGet
			}
			r := httptest.NewRequestWithContext(t.Context(), method, "http://"+testAddr+tc.target, nil)
			if tc.host != "" {
				r.Host = tc.host
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.cookie != nil {
				r.AddCookie(tc.cookie)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d", w.Code, tc.want)
			}
			if tc.location != "" {
				if got := w.Header().Get("Location"); got != tc.location {
					t.Errorf("Location = %q, want %q", got, tc.location)
				}
				set := w.Header().Get("Set-Cookie")
				for _, want := range []string{"lo0-7410=" + testToken, "HttpOnly", "SameSite=Strict"} {
					if !strings.Contains(set, want) {
						t.Errorf("Set-Cookie %q lacks %q", set, want)
					}
				}
			}
			if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
				t.Errorf("Content-Security-Policy = %q", csp)
			}
		})
	}
}
