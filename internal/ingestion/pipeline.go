package ingestion

import (
	"context"
	"errors"
	"sync"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var ErrOverloaded = errors.New("ingestion is temporarily at capacity")
var ErrBatchTooLarge = errors.New("batch exceeds the ingestion memory budget")

type Sink interface {
	Ingest(context.Context, *collectortracepb.ExportTraceServiceRequest) (Result, error)
}

type QueueStats struct {
	PendingBatches int   `json:"pending_batches"`
	PendingBytes   int64 `json:"pending_bytes"`
}

type Pipeline struct {
	sink       Sink
	maxBatches int
	maxBytes   int64
	mu         sync.Mutex
	pending    QueueStats
}

func NewPipeline(sink Sink, maxBatches int, maxBytes int64) (*Pipeline, error) {
	if sink == nil || maxBatches <= 0 || maxBytes <= 0 {
		return nil, errors.New("ingestion requires a sink and positive queue limits")
	}
	return &Pipeline{sink: sink, maxBatches: maxBatches, maxBytes: maxBytes}, nil
}

// Submit holds its reservation until the sink finishes; acceptance is never a queue-only acknowledgement.
func (p *Pipeline) Submit(ctx context.Context, request *collectortracepb.ExportTraceServiceRequest) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if request == nil {
		return Result{}, errors.New("missing telemetry request")
	}
	size, err := estimateRequest(ctx, request.ProtoReflect(), p.maxBytes)
	if err != nil {
		return Result{}, err
	}
	p.mu.Lock()
	if p.pending.PendingBatches >= p.maxBatches || size > p.maxBytes-p.pending.PendingBytes {
		p.mu.Unlock()
		return Result{}, ErrOverloaded
	}
	p.pending.PendingBatches++
	p.pending.PendingBytes += size
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.pending.PendingBatches--
		p.pending.PendingBytes -= size
		p.mu.Unlock()
	}()
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return p.sink.Ingest(ctx, request)
}

func (p *Pipeline) Snapshot() QueueStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pending
}

// Estimate message objects, list slots and copied values for the request plus the writer's owned clone; this is not peak RSS.
func estimateRequest(ctx context.Context, root protoreflect.Message, maximum int64) (int64, error) {
	var size int64
	stack := []protoreflect.Message{}
	charge := func(bytes int64) bool {
		if bytes < 0 || bytes > maximum-size {
			return false
		}
		size += bytes
		return true
	}
	addMessage := func(message protoreflect.Message) bool {
		if !message.IsValid() {
			return true
		}
		if !charge(512 + 2*int64(len(message.GetUnknown()))) {
			return false
		}
		stack = append(stack, message)
		return true
	}
	addValue := func(kind protoreflect.Kind, value protoreflect.Value) bool {
		switch kind {
		case protoreflect.MessageKind, protoreflect.GroupKind:
			return addMessage(value.Message())
		case protoreflect.StringKind:
			return charge(2 * int64(len(value.String())))
		case protoreflect.BytesKind:
			return charge(2 * int64(len(value.Bytes())))
		}
		return true
	}
	if !addMessage(root) {
		return 0, ErrBatchTooLarge
	}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		message := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		withinBudget := true
		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			if field.IsList() {
				list := value.List()
				withinBudget = charge(16 * int64(list.Len()))
				for i := 0; withinBudget && i < list.Len(); i++ {
					withinBudget = addValue(field.Kind(), list.Get(i))
				}
			} else {
				withinBudget = addValue(field.Kind(), value)
			}
			return withinBudget
		})
		if !withinBudget {
			return 0, ErrBatchTooLarge
		}
	}
	return size, nil
}
