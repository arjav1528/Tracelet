package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tracelet.local/tracelet/internal/collector"
	"tracelet.local/tracelet/internal/config"
	"tracelet.local/tracelet/internal/session"
	"tracelet.local/tracelet/internal/storage"
)

type health struct {
	Version    int    `json:"version"`
	ProjectID  string `json:"project_id"`
	InstanceID string `json:"instance_id"`
}

type Server struct {
	server     *http.Server
	host       string
	health     health
	done       chan struct{}
	serveErr   error
	config     config.Config
	role       string
	token      string
	store      *storage.Store
	receiver   *collector.Receiver
	slots      chan struct{}
	apiTimeout time.Duration
}

// Start binds and retains a loopback listener before returning its actual URL.
func Start(cfg config.Config, d session.Descriptor, store *storage.Store, receiver *collector.Receiver) (*Server, error) {
	if cfg.UI.Bind != "127.0.0.1" || cfg.UI.Port < 0 || cfg.UI.Port > 65535 {
		return nil, errors.New("UI requires a valid loopback endpoint")
	}
	if d.Version != 1 || d.ProjectID != cfg.Project.ID {
		return nil, errors.New("UI session does not match project configuration")
	}
	for _, identity := range []string{d.ProjectID, d.InstanceID} {
		decoded, err := hex.DecodeString(identity)
		if err != nil || len(decoded) != 16 || identity != strings.ToLower(identity) {
			return nil, errors.New("UI requires a valid public session identity")
		}
	}
	nonce, err := hex.DecodeString(d.Nonce)
	if err != nil || len(nonce) != 32 || d.Nonce != strings.ToLower(d.Nonce) || (d.Role != "history" && d.Role != "run") {
		return nil, errors.New("UI requires a valid private session identity and role")
	}
	l := cfg.Limits
	for _, seconds := range []int{l.ReadHeaderTimeoutSeconds, l.ReadTimeoutSeconds, l.WriteTimeoutSeconds, l.IdleTimeoutSeconds} {
		if seconds <= 0 || seconds > 3600 {
			return nil, errors.New("UI timeouts must be between 1 and 3600 seconds")
		}
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(cfg.UI.Bind, strconv.Itoa(cfg.UI.Port)))
	if err != nil {
		return nil, err
	}
	concurrency := 8
	if cfg.Limits.MaxConcurrentRequests > 0 {
		concurrency = min(concurrency, cfg.Limits.MaxConcurrentRequests)
	}
	s := &Server{host: listener.Addr().String(), health: health{Version: 1, ProjectID: d.ProjectID, InstanceID: d.InstanceID}, done: make(chan struct{}), config: cfg, role: d.Role, token: d.Nonce, store: store, receiver: receiver, slots: make(chan struct{}, concurrency), apiTimeout: time.Duration(min(10, l.WriteTimeoutSeconds)) * time.Second}
	s.server = &http.Server{
		Handler: s, ReadHeaderTimeout: time.Duration(l.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout: time.Duration(l.ReadTimeoutSeconds) * time.Second, WriteTimeout: time.Duration(l.WriteTimeoutSeconds) * time.Second,
		IdleTimeout: time.Duration(l.IdleTimeoutSeconds) * time.Second, MaxHeaderBytes: 16 << 10,
	}
	go func() {
		err := s.server.Serve(listener)
		if !errors.Is(err, http.ErrServerClosed) {
			s.serveErr = err
		}
		close(s.done)
	}()
	return s, nil
}

func (s *Server) URL() string { return "http://" + s.host }

func (s *Server) Close(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, s.server.Close())
	}
	select {
	case <-s.done:
		return errors.Join(err, s.serveErr)
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	origins, sites := request.Header.Values("Origin"), request.Header.Values("Sec-Fetch-Site")
	api := request.URL.Path == "/api/v1" || strings.HasPrefix(request.URL.Path, "/api/v1/")
	if request.Host != s.host || len(origins) > 1 || (len(origins) == 1 && origins[0] != s.URL()) || len(sites) > 1 || (len(sites) == 1 && sites[0] != "same-origin" && sites[0] != "none") {
		if api {
			writeAPIError(w, http.StatusForbidden, "forbidden", "request must come from this local Tracelet instance")
			return
		}
		http.Error(w, "request must come from this local Tracelet instance", http.StatusForbidden)
		return
	}
	if api {
		s.serveAPI(w, request)
		return
	}
	if request.URL.Path != "/" && request.URL.Path != "/healthz" {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	if request.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "request requires GET", http.StatusMethodNotAllowed)
		return
	}
	if request.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.health)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, "<!doctype html><html lang=\"en\"><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>Tracelet</title><main><h1>Tracelet</h1><p>Your local trace history session is running.</p></main></html>")
}
