package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	if err := serve(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve() error {
	address := flag.String("addr", "127.0.0.1:8080", "Sample HTTP listen address")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", "tracelet-go-api")), resource.WithFromEnv())
	if err != nil {
		return err
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	if err != nil {
		return err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(exporter))
	defer func() {
		flush, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := provider.Shutdown(flush); err != nil {
			fmt.Fprintln(os.Stderr, "flush telemetry:", err)
		}
	}()
	tracer := provider.Tracer("tracelet.example.go-api")
	downstreamListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	downstream := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parent := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		_, span := tracer.Start(parent, "payment service", trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		w.WriteHeader(http.StatusNoContent)
	})}
	defer downstream.Close()
	go func() { _ = downstream.Serve(downstreamListener) }()
	listener, err := net.Listen("tcp4", *address)
	if err != nil {
		return err
	}
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: orderHandler(tracer, "http://"+downstreamListener.Addr().String())}
	defer server.Close()
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	fmt.Printf("Sample API: http://%s/orders/123 (error example: /orders/error)\n", listener.Addr())
	select {
	case err := <-finished:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return err
		}
	}
	return nil
}

func orderHandler(tracer trace.Tracer, downstream string) http.Handler {
	client := &http.Client{Timeout: time.Second}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || (r.URL.Path != "/orders/123" && r.URL.Path != "/orders/error") {
			http.NotFound(w, r)
			return
		}
		parent := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, request := tracer.Start(parent, "GET /orders/{id}", trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", "GET"), attribute.String("http.route", "/orders/{id}")))
		request.SetAttributes(sourceAttributes()...)
		defer request.End()
		_, auth := tracer.Start(ctx, "authenticate", trace.WithAttributes(sourceAttributes()...))
		auth.End()
		_, lookup := tracer.Start(ctx, "GetOrder", trace.WithAttributes(sourceAttributes()...))
		select {
		case <-time.After(80 * time.Millisecond):
		case <-ctx.Done():
		}
		lookup.AddEvent("synthetic order lookup")
		if r.URL.Path == "/orders/error" {
			err := errors.New("synthetic order failure")
			lookup.RecordError(err)
			lookup.SetStatus(codes.Error, "synthetic failure")
			lookup.End()
			request.SetStatus(codes.Error, "synthetic failure")
			request.SetAttributes(attribute.Int("http.response.status_code", 500))
			http.Error(w, "synthetic failure", 500)
			return
		}
		lookup.End()
		outboundCtx, payment := tracer.Start(ctx, "payment API", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(sourceAttributes()...))
		req, err := http.NewRequestWithContext(outboundCtx, http.MethodGet, downstream, nil)
		if err == nil {
			propagation.TraceContext{}.Inject(outboundCtx, propagation.HeaderCarrier(req.Header))
			var response *http.Response
			response, err = client.Do(req)
			if response != nil {
				_ = response.Body.Close()
				if response.StatusCode >= 400 {
					err = fmt.Errorf("synthetic downstream status %d", response.StatusCode)
				}
			}
		}
		if err != nil {
			payment.RecordError(err)
			payment.SetStatus(codes.Error, "payment unavailable")
		}
		payment.End()
		if err != nil {
			request.SetStatus(codes.Error, "payment unavailable")
			http.Error(w, "payment unavailable", 502)
			return
		}
		request.SetAttributes(attribute.Int("http.response.status_code", 200))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"id\":\"123\",\"status\":\"synthetic\"}\n"))
	})
}

func sourceAttributes() []attribute.KeyValue {
	pc, file, line, ok := runtime.Caller(1)
	if !ok {
		return nil
	}
	function := ""
	if f := runtime.FuncForPC(pc); f != nil {
		function = f.Name()
	}
	return []attribute.KeyValue{attribute.String("code.file.path", file), attribute.String("code.function.name", function), attribute.Int("code.line.number", line)}
}
