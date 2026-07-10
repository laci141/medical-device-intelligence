package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/laci141/medical-device-intelligence/internal/cliutil"
)

func init() { register("serve", cmdServe) }

// moduleCount is the number of intelligence modules exposed by the platform
// (01 Telemetry .. 12 Synthesis). There is no module registry to derive it from.
const moduleCount = 12

// getenv is an indirection so tests can fake the PORT environment variable
// without mutating the real process environment.
var getenv = os.Getenv

// cmdServe starts an HTTP server that exposes the command surface as JSON
// endpoints (same-origin API for a future HTML frontend). Each endpoint is a
// thin adapter over the existing command handlers run in --json mode, so the
// envelope shape, the disclaimer, and the never-a-risk-score conventions are
// inherited rather than re-implemented. On Render-style platforms the PORT
// environment variable, when set, overrides --port.
func cmdServe(ctx context.Context, stdout, stderr io.Writer, args []string) int {
	fs, _ := newFlagSet("serve")
	port := fs.Int("port", 8080, "listen port (PORT env, when set, wins)")
	fs.String("db", "", "path to the SQLite cache (reserved for cached endpoints)")
	if err := parse(fs, stderr, args, map[string]bool{"port": true, "db": true}); err != nil {
		return 2
	}
	p := *port
	if env := getenv("PORT"); env != "" {
		n, err := strconv.Atoi(env)
		if err != nil {
			fmt.Fprintf(stderr, "serve: PORT env is not a number: %q\n", env)
			return 2
		}
		p = n
	}
	if p < 1 || p > 65535 {
		fmt.Fprintf(stderr, "serve: port must be 1-65535, got %d\n", p)
		return 2
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", p),
		Handler:           NewServeHandler(),
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	fmt.Fprintf(stderr, "serve: listening on :%d (Ctrl+C to stop)\n", p)

	select {
	case <-ctx.Done():
		// Graceful shutdown: stop accepting, let in-flight requests finish.
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			fmt.Fprintf(stderr, "serve: shutdown: %v\n", err)
			return 1
		}
		fmt.Fprintln(stderr, "serve: stopped")
		return 0
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "serve: %v\n", err)
			return 1
		}
		return 0
	}
}

// apiRoute maps one GET endpoint onto a registered command run in --json mode.
type apiRoute struct {
	// params are the required query parameters, checked before dispatch so a
	// missing one is a clean 400 rather than a command usage message.
	params []string
	// argv builds the command argv (including the command name) from the query.
	argv func(q url.Values) []string
}

var apiRoutes = map[string]apiRoute{
	"/api/search": {
		params: []string{"device"},
		argv:   func(q url.Values) []string { return []string{"search", q.Get("device"), "--json"} },
	},
	"/api/signals": {
		params: []string{"device"},
		argv:   func(q url.Values) []string { return []string{"signals", "--device", q.Get("device"), "--json"} },
	},
	"/api/dossier": {
		params: []string{"device"},
		argv:   func(q url.Values) []string { return []string{"dossier", "--device", q.Get("device"), "--json"} },
	},
	"/api/compare": {
		params: []string{"a", "b"},
		argv:   func(q url.Values) []string { return []string{"compare", q.Get("a"), q.Get("b"), "--json"} },
	},
}

// NewServeHandler builds the API handler. Exported for the frontend embed step
// later; tests drive it through httptest.
func NewServeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", handleHealth)
	for path, route := range apiRoutes {
		mux.HandleFunc(path, routeHandler(route))
	}
	// Everything else (including "/" until the frontend lands) is a JSON 404.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, http.StatusNotFound, "not found")
	})
	return withRecovery(mux)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if !allowGET(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"commands":   len(commands),
		"modules":    moduleCount,
		"disclaimer": cliutil.Disclaimer,
	})
}

// routeHandler adapts one command to HTTP: required params → 400, command usage
// error (exit 2) → 400, command runtime/upstream error (exit 1) → 502, success →
// the command's own --json envelope (disclaimer included) passed through.
func routeHandler(route apiRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !allowGET(w, r) {
			return
		}
		q := r.URL.Query()
		for _, p := range route.params {
			if strings.TrimSpace(q.Get(p)) == "" {
				writeJSONError(w, http.StatusBadRequest, p+" required")
				return
			}
		}
		var out, errBuf bytes.Buffer
		code := Dispatch(r.Context(), &out, &errBuf, route.argv(q))
		switch code {
		case 0:
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(out.Bytes())
		case 2:
			writeJSONError(w, http.StatusBadRequest, firstLine(errBuf.String()))
		default:
			writeJSONError(w, http.StatusBadGateway, firstLine(errBuf.String()))
		}
	}
}

// allowGET rejects non-GET methods with a JSON 405. Only safe GETs are served,
// which is also why the wide-open CORS header is acceptable.
func allowGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet {
		return true
	}
	w.Header().Set("Allow", http.MethodGet)
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

// withRecovery converts a handler panic into a JSON 500 instead of killing the
// connection: module errors must never take the server down.
func withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("internal error: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"error":      msg,
		"disclaimer": cliutil.Disclaimer,
	})
}

// firstLine trims a multi-line stderr capture to its first line for the JSON
// error field; the full text stays server-side only.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if s == "" {
		s = "command failed"
	}
	return s
}
