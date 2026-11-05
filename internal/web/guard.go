package web

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
)

// guard admits only requests that name this server's own address and carry
// the token. A token in the URL is moved into a cookie and dropped from the
// address, so it does not linger in history. Forms must come from lo0's own
// pages.
func guard(addr, token string, next http.Handler) http.Handler {
	hosts := map[string]bool{addr: true}
	if _, port, err := net.SplitHostPort(addr); err == nil {
		hosts[net.JoinHostPort("localhost", port)] = true
		hosts[net.JoinHostPort("127.0.0.1", port)] = true
	}
	// Browsers keep cookies per host, not per port, so each address gets
	// its own name and two copies of lo0 do not sign each other out.
	cookie := "lo0"
	if _, port, err := net.SplitHostPort(addr); err == nil {
		cookie += "-" + port
	}
	same := func(s string) bool { return subtle.ConstantTimeCompare([]byte(s), []byte(token)) == 1 }

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		// same-origin, not no-referrer: with no-referrer browsers send
		// "Origin: null" on form posts, which the check below must refuse.
		// The token never stays in an address, so nothing leaks either way.
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Content-Type-Options", "nosniff")

		if !hosts[r.Host] {
			http.Error(w, "lo0 answers only at the address it printed.", http.StatusMisdirectedRequest)
			return
		}
		if q := r.URL.Query(); q.Has("token") {
			if !same(q.Get("token")) {
				denied(w)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: cookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			q.Del("token")
			clean := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
			http.Redirect(w, r, clean.String(), http.StatusSeeOther)
			return
		}
		if c, err := r.Cookie(cookie); err != nil || !same(c.Value) {
			denied(w)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "lo0 accepts changes only from its own pages.", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// denied answers a request without the token.
func denied(w http.ResponseWriter) {
	http.Error(w, "Open lo0 from the link lo0 web printed in your terminal.", http.StatusForbidden)
}
