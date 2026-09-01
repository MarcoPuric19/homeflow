package httpd

import (
	"net/http"
	"strings"
)

// sameOriginOnly protects every request that changes something.
//
// Mission Control has no authentication - it binds to loopback and trusts
// whoever sits at the machine. That is fine for reading, but a *browser* is not
// only used by the person at the machine: any web page can make the browser
// send a cross-origin POST to 127.0.0.1. Without this guard, visiting a random
// site could create tickets or, once the deploy service exists, rebuild the
// cluster.
//
// The rule keeps non-browser clients working: curl and scripts send neither
// Origin nor Sec-Fetch-Site, and browsers always send Origin on cross-origin
// requests.
func sameOriginOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !isOwnOrigin(origin, r.Host) {
			writeError(w, http.StatusForbidden,
				"Fremde Herkunft abgelehnt. Mission Control nimmt Änderungen nur von der eigenen Oberfläche oder von lokalen Werkzeugen an.")
			return
		}
		// Sec-Fetch-Site is sent by current browsers on every request; "none"
		// means the user typed the address, "same-origin" means our own page.
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			writeError(w, http.StatusForbidden, "Fremde Herkunft abgelehnt.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isOwnOrigin(origin, host string) bool {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(origin, "http://"), "https://")
	return trimmed == host
}
