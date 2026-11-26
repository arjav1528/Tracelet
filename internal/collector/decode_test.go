package collector

import (
	"errors"
	"testing"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"tracelet.local/tracelet/internal/ingestion"
	"tracelet.local/tracelet/testdata/fixtures"
)

func TestDecodePreflightPreservesUnknownFieldsAndBoundsGroups(t *testing.T) {
	valid := fixtures.Protocol("valid").Body
	unknown := protowire.AppendTag(append([]byte(nil), valid...), 100, protowire.BytesType)
	unknown = protowire.AppendBytes(unknown, []byte("synthetic future field"))
	group := protowire.AppendTag(append([]byte(nil), valid...), 101, protowire.StartGroupType)
	group = protowire.AppendTag(group, 102, protowire.VarintType)
	group = protowire.AppendVarint(group, 1)
	group = protowire.AppendTag(group, 101, protowire.EndGroupType)
	for _, payload := range [][]byte{valid, unknown, group} {
		if err := checkDecodeBudget(payload, 32<<20, 40); err != nil {
			t.Fatal(err)
		}
		message := &collectortracepb.ExportTraceServiceRequest{}
		if err := (proto.UnmarshalOptions{RecursionLimit: 40}).Unmarshal(payload, message); err != nil {
			t.Fatalf("preflight accepted invalid protobuf: %v", err)
		}
		if spanCount(message) != 3 {
			t.Fatal("unknown fields changed spans")
		}
	}
	deep := []byte{}
	for i := 0; i < 50; i++ {
		deep = protowire.AppendTag(deep, 100, protowire.StartGroupType)
	}
	for i := 0; i < 50; i++ {
		deep = protowire.AppendTag(deep, 100, protowire.EndGroupType)
	}
	for name, payload := range map[string][]byte{
		"deep groups":        deep,
		"unterminated group": protowire.AppendTag(nil, 100, protowire.StartGroupType),
		"wrong group end":    protowire.AppendTag(protowire.AppendTag(nil, 100, protowire.StartGroupType), 101, protowire.EndGroupType),
		"zero tag":           {0}, "truncated length": {10, 127, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := checkDecodeBudget(payload, 32<<20, 40); !errors.Is(err, errMalformedProtobuf) {
				t.Fatalf("malformed input returned %v", err)
			}
		})
	}
	if err := checkDecodeBudget(valid, 1024, 40); !errors.Is(err, ingestion.ErrBatchTooLarge) {
		t.Fatalf("allocation budget was not enforced: %v", err)
	}
}
