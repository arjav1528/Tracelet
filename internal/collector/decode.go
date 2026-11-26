package collector

import (
	"errors"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
	"tracelet.local/tracelet/internal/ingestion"
)

var errMalformedProtobuf = errors.New("malformed telemetry Protobuf")

// Small encoded messages can allocate far more than their wire size. Walk the
// descriptor without allocating generated messages, charging the same conservative
// object/value budget as ingestion before decoding. The bounded raw body is separate.
func checkDecodeBudget(body []byte, maximum int64, maxDepth int) error {
	remaining := maximum
	charge := func(size int64) error {
		if size < 0 || size > remaining {
			return ingestion.ErrBatchTooLarge
		}
		remaining -= size
		return nil
	}
	var walk func([]byte, protoreflect.MessageDescriptor, int) error
	walk = func(data []byte, descriptor protoreflect.MessageDescriptor, depth int) error {
		if depth > maxDepth {
			return errMalformedProtobuf
		}
		if err := charge(512); err != nil {
			return err
		}
		for len(data) > 0 {
			number, typ, tagSize := protowire.ConsumeTag(data)
			if tagSize < 0 {
				return errMalformedProtobuf
			}
			data = data[tagSize:]
			field := descriptor.Fields().ByNumber(number)
			if field != nil && field.IsList() {
				if err := charge(16); err != nil {
					return err
				}
			}
			if field != nil && field.Message() != nil && typ == protowire.BytesType {
				value, size := protowire.ConsumeBytes(data)
				if size < 0 {
					return errMalformedProtobuf
				}
				if err := walk(value, field.Message(), depth+1); err != nil {
					return err
				}
				data = data[size:]
				continue
			}
			size := consumeValue(number, typ, data, maxDepth-depth)
			if size < 0 {
				return errMalformedProtobuf
			}
			// Raw unknown fields and scalar values cost no more than twice their
			// encoded size. Charging tags too deliberately overestimates this part.
			if err := charge(2 * int64(tagSize+size)); err != nil {
				return err
			}
			data = data[size:]
		}
		return nil
	}
	return walk(body, (&collectortracepb.ExportTraceServiceRequest{}).ProtoReflect().Descriptor(), 0)
}

// OTLP defines no group fields, but unknown legacy Protobuf groups still need
// depth-limited skipping instead of the library's much larger generic limit.
func consumeValue(number protowire.Number, typ protowire.Type, data []byte, depth int) int {
	if typ != protowire.StartGroupType {
		if typ == protowire.EndGroupType {
			return -1
		}
		return protowire.ConsumeFieldValue(number, typ, data)
	}
	if depth <= 0 {
		return -1
	}
	consumed := 0
	for len(data) > 0 {
		child, childType, size := protowire.ConsumeTag(data)
		if size < 0 {
			return -1
		}
		consumed += size
		data = data[size:]
		if childType == protowire.EndGroupType {
			if child != number {
				return -1
			}
			return consumed
		}
		size = consumeValue(child, childType, data, depth-1)
		if size < 0 {
			return -1
		}
		consumed += size
		data = data[size:]
	}
	return -1
}
