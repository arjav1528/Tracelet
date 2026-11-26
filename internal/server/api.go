package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/arjav1528/Tracelet/internal/collector"
	"github.com/arjav1528/Tracelet/internal/storage"
	"github.com/arjav1528/Tracelet/internal/trace"
)

const maxAPIResponseBytes = 32 << 20

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type statusResponse struct {
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	Session struct {
		Role       string `json:"role"`
		InstanceID string `json:"instance_id"`
	} `json:"session"`
	Receiver collector.Stats `json:"receiver"`
	Storage  storage.Usage   `json:"storage"`
	Settings struct {
		DefaultServiceName  string `json:"default_service_name"`
		Protocol            string `json:"protocol"`
		RetentionDays       int    `json:"retention_days"`
		StorageMaxSizeMB    int64  `json:"storage_max_size_mb"`
		CaptureHTTPBodies   bool   `json:"capture_http_bodies"`
		RedactionEnabled    bool   `json:"redaction_enabled"`
		MaxWireBytes        int64  `json:"max_wire_bytes"`
		MaxDecodedBytes     int64  `json:"max_decoded_bytes"`
		MaxSpansPerBatch    int    `json:"max_spans_per_batch"`
		MaxConcurrentReads  int    `json:"max_concurrent_reads"`
		ReadDeadlineSeconds int    `json:"read_deadline_seconds"`
		MaxResponseBytes    int    `json:"max_response_bytes"`
		MaxTracePage        int    `json:"max_trace_page"`
		MaxTimelinePage     int    `json:"max_timeline_page"`
	} `json:"settings"`
}

func (s *Server) serveAPI(w http.ResponseWriter, request *http.Request) {
	tokens := request.Header.Values("X-Tracelet-Token")
	if len(tokens) != 1 || subtle.ConstantTimeCompare([]byte(tokens[0]), []byte(s.token)) != 1 {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized", "a valid session token is required")
		return
	}
	if request.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "request requires GET")
		return
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		w.Header().Set("Retry-After", "1")
		writeAPIError(w, http.StatusServiceUnavailable, "unavailable", "local queries are temporarily at capacity")
		return
	}
	if s.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "unavailable", "local storage is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), s.apiTimeout)
	defer cancel()
	path := strings.TrimPrefix(request.URL.Path, "/api/v1")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	var result any
	var err error
	switch {
	case path == "/status":
		if _, err = parseQuery(request.URL); err == nil {
			result, err = s.status(ctx)
		}
	case path == "/traces":
		var query url.Values
		query, err = parseQuery(request.URL, "limit", "cursor", "q", "sort")
		if err != nil {
			break
		}
		var limit int
		limit, err = queryLimit(query, 50, 200)
		if err == nil {
			result, err = s.store.ListTraces(ctx, trace.TraceQuery{Limit: limit, Cursor: query.Get("cursor"), Q: query.Get("q"), Sort: query.Get("sort")})
		}
	case len(parts) == 2 && parts[0] == "traces":
		if _, err = parseQuery(request.URL); err == nil {
			result, err = s.store.Trace(ctx, parts[1])
		}
	case len(parts) == 3 && parts[0] == "traces" && parts[2] == "spans":
		var query url.Values
		query, err = parseQuery(request.URL, "limit", "cursor")
		if err != nil {
			break
		}
		var limit int
		limit, err = queryLimit(query, 500, 2000)
		if err == nil {
			result, err = s.store.TraceSpans(ctx, parts[1], query.Get("cursor"), limit)
		}
	case len(parts) == 4 && parts[0] == "traces" && parts[2] == "spans":
		if _, err = parseQuery(request.URL); err == nil {
			result, err = s.store.Span(ctx, parts[1], parts[3])
		}
	default:
		err = storage.ErrNotFound
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		writeQueryError(w, err)
		return
	}
	writeJSON(w, result)
}

func parseQuery(u *url.URL, allowed ...string) (url.Values, error) {
	if len(u.RawQuery) > 8192 || u.RawPath != "" {
		return nil, storage.ErrInvalidQuery
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, storage.ErrInvalidQuery
	}
	for key, values := range query {
		known := false
		for _, name := range allowed {
			known = known || key == name
		}
		if !known || len(values) != 1 || !utf8.ValidString(values[0]) || strings.ContainsRune(values[0], 0) {
			return nil, storage.ErrInvalidQuery
		}
	}
	if len(query.Get("q")) > 256 || len(query.Get("cursor")) > 4096 {
		return nil, storage.ErrInvalidQuery
	}
	return query, nil
}

func queryLimit(query url.Values, fallback, maximum int) (int, error) {
	value, ok := query["limit"]
	if !ok {
		return fallback, nil
	}
	limit, err := strconv.Atoi(value[0])
	if err != nil || limit < 1 || limit > maximum {
		return 0, storage.ErrInvalidQuery
	}
	return limit, nil
}

func (s *Server) status(ctx context.Context) (statusResponse, error) {
	var result statusResponse
	usage, err := s.store.Usage(ctx)
	if err != nil {
		return result, err
	}
	result.Project.ID, result.Project.Name = s.health.ProjectID, s.config.Project.Name
	result.Session.Role, result.Session.InstanceID = s.role, s.health.InstanceID
	if s.receiver != nil {
		result.Receiver = s.receiver.Snapshot()
	}
	result.Storage = usage
	result.Settings.DefaultServiceName = s.config.Project.Name
	result.Settings.Protocol = s.config.Collector.Protocol
	result.Settings.RetentionDays = s.config.Storage.RetentionDays
	result.Settings.StorageMaxSizeMB = s.config.Storage.MaxSizeMB
	result.Settings.CaptureHTTPBodies = false
	result.Settings.RedactionEnabled = true
	result.Settings.MaxWireBytes = s.config.Limits.MaxWireBytes
	result.Settings.MaxDecodedBytes = s.config.Limits.MaxDecodedBytes
	result.Settings.MaxSpansPerBatch = s.config.Limits.MaxSpansPerBatch
	result.Settings.MaxConcurrentReads = cap(s.slots)
	result.Settings.ReadDeadlineSeconds = int(s.apiTimeout.Seconds())
	result.Settings.MaxResponseBytes = maxAPIResponseBytes
	result.Settings.MaxTracePage, result.Settings.MaxTimelinePage = 200, 2000
	return result, nil
}

func writeQueryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrInvalidQuery):
		writeAPIError(w, http.StatusBadRequest, "invalid_query", "request path or query is invalid")
	case errors.Is(err, storage.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "requested trace, span or route was not found")
	case errors.Is(err, storage.ErrRevisionChanged):
		writeAPIError(w, http.StatusConflict, "revision_changed", "trace data changed; restart pagination")
	case errors.Is(err, context.DeadlineExceeded):
		writeAPIError(w, http.StatusGatewayTimeout, "request_timeout", "local query exceeded its deadline")
	case errors.Is(err, context.Canceled):
		writeAPIError(w, http.StatusRequestTimeout, "request_canceled", "local query was canceled")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "local data could not be read")
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "local response could not be encoded")
		return
	}
	if len(data) >= maxAPIResponseBytes {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "response_too_large", "local response exceeds the supported size")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(data, '\n'))
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	var response apiError
	response.Error.Code, response.Error.Message = code, message
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
