package supervisor

import (
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentPrecedence(t *testing.T) {
	const endpoint = "http://127.0.0.1:4319/v1/traces"
	cases := []struct {
		name                        string
		input                       []string
		override                    bool
		endpoint, protocol, service string
	}{
		{"defaults", []string{"OTHER=unchanged"}, false, endpoint, "http/protobuf", "project"},
		{"generic", []string{"OTEL_EXPORTER_OTLP_ENDPOINT=https://remote.invalid/base", "OTEL_EXPORTER_OTLP_PROTOCOL=grpc"}, false, "", "", "project"},
		{"trace-specific", []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=https://remote.invalid/traces", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL=http/json"}, false, "https://remote.invalid/traces", "http/json", "project"},
		{"endpoint-only", []string{"OTEL_EXPORTER_OTLP_ENDPOINT=https://remote.invalid"}, false, "", "http/protobuf", "project"},
		{"protocol-only", []string{"OTEL_EXPORTER_OTLP_PROTOCOL=grpc"}, false, endpoint, "", "project"},
		{"empty-is-unset", []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL="}, false, endpoint, "http/protobuf", "project"},
		{"service", []string{"OTEL_SERVICE_NAME=explicit"}, false, endpoint, "http/protobuf", "explicit"},
		{"resource-service", []string{"OTEL_RESOURCE_ATTRIBUTES=region=local,service.name=resource-name"}, false, endpoint, "http/protobuf", ""},
		{"override-traces-only", []string{"OTEL_EXPORTER_OTLP_ENDPOINT=https://remote.invalid", "OTEL_EXPORTER_OTLP_PROTOCOL=grpc", "OTEL_SERVICE_NAME=explicit"}, true, endpoint, "http/protobuf", "explicit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]string(nil), tc.input...)
			values, _ := Env(tc.input, endpoint, "project", tc.override)
			for key, want := range map[string]string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": tc.endpoint, "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": tc.protocol, "OTEL_SERVICE_NAME": tc.service} {
				if got := lookupTest(values, key); got != want {
					t.Fatalf("%s=%q want %q", key, got, want)
				}
			}
			if !reflect.DeepEqual(tc.input, original) {
				t.Fatal("mutated parent environment")
			}
			for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_RESOURCE_ATTRIBUTES", "OTHER"} {
				if lookupTest(values, key) != lookupTest(tc.input, key) {
					t.Fatalf("modified %s", key)
				}
			}
		})
	}
}

func TestEnvironmentDoesNotExposeSecretsOrEnableSDK(t *testing.T) {
	parent := []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=https://user:PRIVATE@remote.invalid/PRIVATE?token=PRIVATE", "OTEL_EXPORTER_OTLP_HEADERS=authorization=PRIVATE", "OTEL_SDK_DISABLED=true", "OTEL_TRACES_EXPORTER=none", "OTEL_TRACES_SAMPLER=always_off"}
	values, notices := Env(parent, "http://127.0.0.1:4319/v1/traces", "project", false)
	for _, key := range []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_HEADERS", "OTEL_SDK_DISABLED", "OTEL_TRACES_EXPORTER", "OTEL_TRACES_SAMPLER"} {
		if lookupTest(values, key) != lookupTest(parent, key) {
			t.Fatalf("modified %s", key)
		}
	}
	joined := strings.Join(notices, "\n")
	if strings.Contains(joined, "PRIVATE") || strings.Contains(joined, "user:") {
		t.Fatalf("notice leaked secret: %s", joined)
	}
	if !strings.Contains(joined, "OTEL_SDK_DISABLED") || !strings.Contains(joined, "OTEL_TRACES_EXPORTER") {
		t.Fatalf("missing disabled diagnostics: %s", joined)
	}
}

func lookupTest(values []string, key string) string {
	for i := len(values) - 1; i >= 0; i-- {
		k, v, ok := strings.Cut(values[i], "=")
		if ok && k == key {
			return v
		}
	}
	return ""
}
