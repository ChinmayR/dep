package health

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// NewPlain constructs a plain-text, HTTP-only health check handler. It
// supports both simple Nagios-style job control checks and the more
// sophisticated checks used by Uber's Health Controller system.
//
// Typically, applications mount this handler on a mux bound to
// $UBER_PORT_SYSTEM.
func NewPlain(hc *Coordinator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		current := hc.State()
		query := r.URL.Query()

		// Always set the health headers.
		w.Header().Set("Health-Status", strings.ToUpper(current.String()))
		w.Header().Set("Health-Service", hc.name)

		// If the caller specifies the target service, make sure the names match.
		if target := query.Get("service"); target != "" && target != hc.name {
			w.WriteHeader(http.StatusForbidden)
			if r.Method != http.MethodHead {
				fmt.Fprintf(w, "check target was service %q, but this is %q", target, hc.name)
			}
			return
		}

		if query.Get("type") != "traffic" {
			serveSimpleHTTP(w, r)
			return
		}
		serveDetailedHTTP(w, r, current)
	})
}

func serveDetailedHTTP(w http.ResponseWriter, r *http.Request, current State) {
	if current != AcceptingTraffic {
		w.WriteHeader(http.StatusNotFound)
	}
	if r.Method == http.MethodHead {
		return
	}
	fmt.Fprintln(w, strings.ToUpper(current.String()))
}

func serveSimpleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodHead {
		return
	}
	io.WriteString(w, "OK")
}
