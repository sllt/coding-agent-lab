// Package bootstrap assembles the Pi control plane. Domain code does not
// depend on *pi.Context; Pi only owns transport and process lifetime.
package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sllt/pi/pkg/pi"
	"github.com/sllt/pi/pkg/pi/http/response"

	"github.com/sllt/agentlab/internal/version"
)

const (
	defaultAddr = "127.0.0.1:43117"
)

// Config is explicit. Build does not read the environment.
type Config struct {
	Addr    string
	DataDir string
	// Register adds routes before the router is compiled.
	Register func(*pi.App)
	// Worker is the long-lived scheduler loop. Trial failures must not be
	// returned; only an unsafe control plane returns an error.
	Worker func(context.Context) error
	// ExtraResources are declared after the HTTP listener, for tests.
	ExtraResources []pi.Resource
	// Wrap sits outside the Pi router and inside the loopback check.
	Wrap func(http.Handler) http.Handler
}

// Runtime is one Pi application plus the host HTTP server.
type Runtime struct {
	App     *pi.App
	Handler http.Handler
	Addr    string

	listenAddr string
	ln         net.Listener
	srv        *http.Server
	requests   *requestIDs
}

type requestIDs struct {
	mu sync.Mutex
	id string
}

// Build constructs the application. It does not listen or open workers until Start.
func Build(cfg Config) (*Runtime, error) {
	if cfg.Addr == "" {
		cfg.Addr = defaultAddr
	}
	if err := requireLoopback(cfg.Addr); err != nil {
		return nil, err
	}
	rt := &Runtime{requests: &requestIDs{}, listenAddr: cfg.Addr}
	resources := []pi.Resource{{
		Name:      "http",
		Ownership: pi.Owned,
		Start:     rt.startHTTP,
		Stop:      rt.stopHTTP,
	}}
	resources = append(resources, cfg.ExtraResources...)
	opts := []pi.Option{
		pi.WithExplicitHTTPStatus(),
		pi.WithConfig(map[string]string{
			"HTTP_ENABLED":           "false",
			"GRPC_ENABLED":           "false",
			"METRICS_ENABLED":        "false",
			"CORS_ALLOWED_ORIGINS":   "",
			"CORS_ALLOW_CREDENTIALS": "false",
		}),
	}
	for _, r := range resources {
		opts = append(opts, pi.WithResource(r))
	}
	app, err := pi.Build(opts...)
	if err != nil {
		return nil, err
	}
	registerHealth(app)
	if cfg.Register != nil {
		cfg.Register(app)
	}
	handler, err := app.HTTPHandler()
	if err != nil {
		return nil, err
	}
	rt.App = app
	if cfg.Wrap != nil {
		handler = cfg.Wrap(handler)
	}
	rt.Handler = secure(cfg.Addr, handler, rt.requests)
	worker := cfg.Worker
	app.Go("scheduler", func(c *pi.Context) error {
		if worker == nil {
			<-c.Done()
			return nil
		}
		err := worker(c)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	})
	return rt, nil
}

func registerHealth(app *pi.App) {
	app.GET("/api/v1/health", func(c *pi.Context) (any, error) {
		return response.Raw{StatusCode: http.StatusOK, Data: map[string]any{
			"data": map[string]string{
				"status":  "ok",
				"service": version.Product,
				"version": version.Version,
			},
			"request_id": requestIDFrom(c.Context),
		}}, nil
	})
	app.GET("/api/v1/health/stream", func(c *pi.Context) (any, error) {
		return response.Stream{
			StatusCode:  http.StatusOK,
			ContentType: "text/event-stream",
			Run: func(ctx context.Context, w io.Writer) error {
				_, err := w.Write([]byte("event: health\ndata: {\"status\":\"ok\"}\n\n"))
				return err
			},
		}, nil
	})
}

func (rt *Runtime) startHTTP(ctx context.Context) error {
	if rt.Handler == nil {
		return errors.New("HTTP handler 尚未编译")
	}
	ln, err := net.Listen("tcp", rt.boundAddr())
	if err != nil {
		return err
	}
	rt.ln = ln
	rt.Addr = ln.Addr().String()
	rt.srv = &http.Server{
		Handler:           rt.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	go func() {
		err := rt.srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			rt.App.Logger().Errorf("http serve: %v", err)
		}
	}()
	return nil
}

func (rt *Runtime) boundAddr() string {
	if rt.listenAddr != "" {
		return rt.listenAddr
	}
	return defaultAddr
}

func (rt *Runtime) stopHTTP(ctx context.Context) error {
	if rt.srv == nil {
		if rt.ln != nil {
			return rt.ln.Close()
		}
		return nil
	}
	return rt.srv.Shutdown(ctx)
}

// Start runs Pi startup. The address is chosen before listen when it is fixed.
func (rt *Runtime) Start(ctx context.Context) error {
	return rt.App.Start(ctx)
}

// Stop shuts the control plane down once.
func (rt *Runtime) Stop(ctx context.Context) error {
	return rt.App.Stop(ctx)
}

type idKey struct{}

func requestIDFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(idKey{}).(string)
	return v
}

// RequestID returns the id attached by the host middleware.
func RequestID(ctx context.Context) string { return requestIDFrom(ctx) }

type statusWriter struct {
	http.ResponseWriter
	id      string
	written bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	w.written = true
	w.Header().Set("X-Request-ID", w.id)
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func secure(listenAddr string, next http.Handler, ids *requestIDs) http.Handler {
	host, port, err := splitHostPortLoose(listenAddr)
	_ = err
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newID()
		if ids != nil {
			ids.mu.Lock()
			ids.id = id
			ids.mu.Unlock()
		}
		ctx := context.WithValue(r.Context(), idKey{}, id)
		r = r.WithContext(ctx)
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		sw := &statusWriter{ResponseWriter: w, id: id}
		if !hostAllowed(r.Host) {
			writeErr(sw, id, http.StatusBadRequest, "invalid_host", "Host 不在允许的本机地址内")
			return
		}
		if r.Header.Get("Origin") != "" && !originAllowed(r.Header.Get("Origin"), host, port, r.Host) {
			writeErr(sw, id, http.StatusForbidden, "invalid_origin", "Origin 不被接受")
			return
		}
		// CORS stays off. A same-origin browser still sends Origin; drop it after
		// the host check so the framework does not reflect an allow-origin header.
		r.Header.Del("Origin")
		next.ServeHTTP(sw, r)
	})
}

func writeErr(w http.ResponseWriter, requestID string, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, "{\"error\":{\"code\":%q,\"message\":%q},\"request_id\":%q}\n", code, message, requestID)
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return "req-" + hex.EncodeToString(b[:])
}

const contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws:; frame-ancestors 'none'; base-uri 'self'; object-src 'none'"

func requireLoopback(addr string) error {
	host, _, err := splitHostPortLoose(addr)
	if err != nil {
		return fmt.Errorf("监听地址无效: %w", err)
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return fmt.Errorf("控制服务只绑定 loopback，拒绝 %s", addr)
	}
	return nil
}

func splitHostPortLoose(addr string) (string, string, error) {
	if strings.HasPrefix(addr, "[") {
		return net.SplitHostPort(addr)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}
	return host, port, nil
}

func hostAllowed(hostport string) bool {
	host, _, err := splitHostPortLoose(hostport)
	if err != nil {
		// httptest sometimes uses a host without a port.
		host = hostport
	}
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

func originAllowed(origin, listenHost, listenPort, requestHost string) bool {
	origin = strings.TrimRight(origin, "/")
	if !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://") {
		return false
	}
	u := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	h, p, err := splitHostPortLoose(u)
	if err != nil {
		h = u
		p = ""
	}
	if !hostAllowed(h) {
		return false
	}
	if listenPort == "0" || p == "" {
		return true
	}
	rh, rp, err := splitHostPortLoose(requestHost)
	if err == nil && h == rh && p == rp {
		return true
	}
	return p == listenPort && (h == listenHost || h == "localhost" || h == "127.0.0.1")
}
