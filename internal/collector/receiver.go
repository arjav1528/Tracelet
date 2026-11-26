package collector

import (
	"compress/gzip"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/arjav1528/Tracelet/internal/ingestion"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	codepb "google.golang.org/genproto/googleapis/rpc/code"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"
)

type Stats struct {
	Running            bool                 `json:"running"`
	Requests           uint64               `json:"requests"`
	ReceivedSpans      uint64               `json:"received_spans"`
	AcceptedSpans      uint64               `json:"accepted_spans"`
	RejectedSpans      uint64               `json:"rejected_spans"`
	RejectedRequests   uint64               `json:"rejected_requests"`
	OverloadedRequests uint64               `json:"overloaded_requests"`
	Duplicates         uint64               `json:"duplicates"`
	Conflicts          uint64               `json:"conflicts"`
	ResponseErrors     uint64               `json:"response_errors"`
	LastReceivedAt     string               `json:"last_received_at,omitempty"`
	LastError          string               `json:"last_error,omitempty"`
	ActiveRequests     int                  `json:"active_requests"`
	Queue              ingestion.QueueStats `json:"queue"`
}

type Receiver struct {
	server   *http.Server
	host     string
	limits   config.Limits
	validate ingestion.Limits
	pipeline *ingestion.Pipeline
	slots    chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	stats    Stats
	serveErr error
}

func Start(cfg config.Config, sink ingestion.Sink) (*Receiver, error) {
	if cfg.Collector.Bind != "127.0.0.1" || cfg.Collector.Port < 0 || cfg.Collector.Port > 65535 || cfg.Collector.Protocol != "otlp-http-protobuf" {
		return nil, errors.New("collector requires a valid loopback HTTP/Protobuf endpoint")
	}
	l := cfg.Limits
	for _, value := range []int64{l.MaxWireBytes, l.MaxDecodedBytes, int64(l.MaxSpansPerBatch), int64(l.MaxConcurrentRequests)} {
		if value <= 0 || value > 1<<40 {
			return nil, errors.New("collector limits must be positive and representable")
		}
	}
	for _, seconds := range []int{l.ReadHeaderTimeoutSeconds, l.ReadTimeoutSeconds, l.WriteTimeoutSeconds, l.IdleTimeoutSeconds} {
		if seconds <= 0 || seconds > 3600 {
			return nil, errors.New("collector timeouts must be between 1 and 3600 seconds")
		}
	}
	validation := ingestion.Limits{Attributes: l.MaxAttributes, KeyBytes: l.MaxKeyBytes, ValueBytes: l.MaxValueBytes, Depth: l.MaxDepth, Events: l.MaxEvents, Links: l.MaxLinks}
	if err := ingestion.ValidateResourceSpans(&tracepb.ResourceSpans{}, validation); err != nil || l.MaxDepth > 64 {
		return nil, errors.New("invalid collector attribute limits")
	}
	pipeline, err := ingestion.NewPipeline(sink, l.MaxPendingBatches, l.MaxPendingBytes)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(cfg.Collector.Bind, strconv.Itoa(cfg.Collector.Port)))
	if err != nil {
		return nil, err
	}
	r := &Receiver{host: listener.Addr().String(), limits: l, validate: validation, pipeline: pipeline, slots: make(chan struct{}, l.MaxConcurrentRequests), done: make(chan struct{}), stats: Stats{Running: true}}
	r.server = &http.Server{
		Handler: r, ReadHeaderTimeout: time.Duration(l.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout: time.Duration(l.ReadTimeoutSeconds) * time.Second, WriteTimeout: time.Duration(l.WriteTimeoutSeconds) * time.Second,
		IdleTimeout: time.Duration(l.IdleTimeoutSeconds) * time.Second, MaxHeaderBytes: 16 << 10,
	}
	go func() {
		err := r.server.Serve(listener)
		r.mu.Lock()
		r.stats.Running = false
		if !errors.Is(err, http.ErrServerClosed) {
			r.serveErr = err
		}
		r.mu.Unlock()
		close(r.done)
	}()
	return r, nil
}

func (r *Receiver) Endpoint() string { return "http://" + r.host + "/v1/traces" }

func (r *Receiver) Close(ctx context.Context) error {
	err := r.server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, r.server.Close())
	}
	select {
	case <-r.done:
		r.mu.Lock()
		defer r.mu.Unlock()
		return errors.Join(err, r.serveErr)
	case <-ctx.Done():
		return errors.Join(err, ctx.Err())
	}
}

func (r *Receiver) Snapshot() Stats {
	r.mu.Lock()
	stats := r.stats
	r.mu.Unlock()
	stats.ActiveRequests = len(r.slots)
	stats.Queue = r.pipeline.Snapshot()
	return stats
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	r.mu.Lock()
	r.stats.Requests++
	r.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if request.Host != r.host || len(request.Header.Values("Origin")) != 0 || request.Header.Get("Sec-Fetch-Site") == "cross-site" {
		r.reject(w, http.StatusForbidden, codepb.Code_PERMISSION_DENIED, "collector accepts local SDK requests only", false)
		return
	}
	if request.URL.Path != "/v1/traces" {
		r.reject(w, http.StatusNotFound, codepb.Code_UNIMPLEMENTED, "unsupported telemetry route", false)
		return
	}
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		r.reject(w, http.StatusMethodNotAllowed, codepb.Code_UNIMPLEMENTED, "trace exports require POST", false)
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/x-protobuf" {
		r.reject(w, http.StatusUnsupportedMediaType, codepb.Code_INVALID_ARGUMENT, "expected application/x-protobuf", false)
		return
	}
	encoding := strings.ToLower(strings.TrimSpace(request.Header.Get("Content-Encoding")))
	if encoding != "" && encoding != "identity" && encoding != "gzip" {
		r.reject(w, http.StatusUnsupportedMediaType, codepb.Code_INVALID_ARGUMENT, "unsupported content encoding", false)
		return
	}
	select {
	case r.slots <- struct{}{}:
		defer func() { <-r.slots }()
	default:
		r.reject(w, http.StatusServiceUnavailable, codepb.Code_UNAVAILABLE, "collector is temporarily at capacity", true)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), time.Duration(r.limits.WriteTimeoutSeconds)*time.Second)
	defer cancel()
	body, err := r.readPayload(w, request, encoding)
	if err != nil {
		var wireLimit *http.MaxBytesError
		if errors.Is(err, ingestion.ErrBatchTooLarge) || errors.As(err, &wireLimit) {
			r.reject(w, http.StatusRequestEntityTooLarge, codepb.Code_RESOURCE_EXHAUSTED, "telemetry exceeds configured payload limits", false)
		} else {
			r.reject(w, http.StatusBadRequest, codepb.Code_INVALID_ARGUMENT, "invalid telemetry body or compression", false)
		}
		return
	}
	batch := &collectortracepb.ExportTraceServiceRequest{}
	decoder := proto.UnmarshalOptions{RecursionLimit: 3*r.limits.MaxDepth + 16}
	if err := checkDecodeBudget(body, r.limits.MaxPendingBytes, decoder.RecursionLimit); err != nil {
		if errors.Is(err, ingestion.ErrBatchTooLarge) {
			r.reject(w, http.StatusRequestEntityTooLarge, codepb.Code_RESOURCE_EXHAUSTED, "batch exceeds configured ingestion memory budget", false)
		} else {
			r.reject(w, http.StatusBadRequest, codepb.Code_INVALID_ARGUMENT, "malformed telemetry Protobuf", false)
		}
		return
	}
	if err := decoder.Unmarshal(body, batch); err != nil {
		r.reject(w, http.StatusBadRequest, codepb.Code_INVALID_ARGUMENT, "malformed telemetry Protobuf", false)
		return
	}
	total := countSpans(batch)
	r.mu.Lock()
	r.stats.ReceivedSpans += uint64(total)
	r.mu.Unlock()
	if total > r.limits.MaxSpansPerBatch {
		r.reject(w, http.StatusRequestEntityTooLarge, codepb.Code_RESOURCE_EXHAUSTED, "telemetry exceeds configured span count", false)
		return
	}
	rejected := r.filter(batch)
	r.mu.Lock()
	r.stats.RejectedSpans += uint64(rejected)
	r.mu.Unlock()
	var result ingestion.Result
	if total > rejected {
		result, err = r.pipeline.Submit(ctx, batch)
		if err != nil {
			if errors.Is(err, ingestion.ErrBatchTooLarge) {
				r.reject(w, http.StatusRequestEntityTooLarge, codepb.Code_RESOURCE_EXHAUSTED, "batch exceeds configured ingestion memory budget", false)
			} else {
				r.reject(w, http.StatusServiceUnavailable, codepb.Code_UNAVAILABLE, "trace ingestion is temporarily unavailable", errors.Is(err, ingestion.ErrOverloaded))
			}
			return
		}
	}
	response := &collectortracepb.ExportTraceServiceResponse{}
	if rejected > 0 {
		response.PartialSuccess = &collectortracepb.ExportTracePartialSuccess{RejectedSpans: int64(rejected), ErrorMessage: "spans rejected by identity, timing, metadata or attribute limits"}
	}
	r.mu.Lock()
	r.stats.AcceptedSpans += uint64(result.Accepted)
	r.stats.Duplicates += uint64(result.Duplicates)
	r.stats.Conflicts += uint64(result.Conflicts)
	r.stats.LastError = ""
	if total > rejected {
		r.stats.LastReceivedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	r.mu.Unlock()
	r.respond(w, http.StatusOK, response)
}

func (r *Receiver) readPayload(w http.ResponseWriter, request *http.Request, encoding string) ([]byte, error) {
	defer request.Body.Close()
	if request.ContentLength > r.limits.MaxWireBytes {
		return nil, ingestion.ErrBatchTooLarge
	}
	var reader io.Reader = http.MaxBytesReader(w, request.Body, r.limits.MaxWireBytes)
	if encoding == "gzip" {
		compressed, err := gzip.NewReader(reader)
		if err != nil {
			return nil, err
		}
		defer compressed.Close()
		reader = compressed
	}
	body, err := io.ReadAll(io.LimitReader(reader, r.limits.MaxDecodedBytes+1))
	if int64(len(body)) > r.limits.MaxDecodedBytes {
		return nil, ingestion.ErrBatchTooLarge
	}
	return body, err
}

func (r *Receiver) filter(batch *collectortracepb.ExportTraceServiceRequest) int {
	rejected := 0
	resources := batch.ResourceSpans[:0]
	for _, resource := range batch.ResourceSpans {
		if resource == nil {
			continue
		}
		shared := &tracepb.ResourceSpans{Resource: resource.Resource, SchemaUrl: resource.SchemaUrl}
		if ingestion.ValidateResourceSpans(shared, r.validate) != nil {
			for _, scope := range resource.ScopeSpans {
				rejected += len(scope.GetSpans())
			}
			continue
		}
		scopes := resource.ScopeSpans[:0]
		for _, scope := range resource.ScopeSpans {
			if scope == nil {
				continue
			}
			shared.ScopeSpans = []*tracepb.ScopeSpans{scope}
			if ingestion.ValidateResourceSpans(shared, r.validate) != nil {
				rejected += len(scope.Spans)
				continue
			}
			spans := scope.Spans[:0]
			for _, span := range scope.Spans {
				if ingestion.ValidateSpan(span, r.validate) != nil {
					rejected++
					continue
				}
				spans = append(spans, span)
			}
			clear(scope.Spans[len(spans):])
			scope.Spans = spans
			if len(spans) > 0 {
				scopes = append(scopes, scope)
			}
		}
		clear(resource.ScopeSpans[len(scopes):])
		resource.ScopeSpans = scopes
		if len(scopes) > 0 {
			resources = append(resources, resource)
		}
	}
	clear(batch.ResourceSpans[len(resources):])
	batch.ResourceSpans = resources
	return rejected
}

func countSpans(batch *collectortracepb.ExportTraceServiceRequest) int {
	count := 0
	for _, resource := range batch.ResourceSpans {
		for _, scope := range resource.GetScopeSpans() {
			count += len(scope.GetSpans())
		}
	}
	return count
}

func (r *Receiver) reject(w http.ResponseWriter, status int, code codepb.Code, message string, overloaded bool) {
	r.mu.Lock()
	r.stats.RejectedRequests++
	r.stats.LastError = message
	if overloaded {
		r.stats.OverloadedRequests++
	}
	r.mu.Unlock()
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "1")
	}
	r.respond(w, status, &statuspb.Status{Code: int32(code), Message: message})
}

func (r *Receiver) respond(w http.ResponseWriter, status int, message proto.Message) {
	w.Header().Set("Content-Type", "application/x-protobuf")
	encoded, err := proto.Marshal(message)
	if err == nil {
		w.WriteHeader(status)
		_, err = w.Write(encoded)
	} else {
		// An empty Protobuf body is also a valid empty google.rpc.Status.
		w.WriteHeader(http.StatusInternalServerError)
	}
	if err != nil {
		r.mu.Lock()
		r.stats.ResponseErrors++
		r.stats.LastError = "telemetry response could not be written"
		r.mu.Unlock()
	}
}
