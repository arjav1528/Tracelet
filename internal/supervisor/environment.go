package supervisor

import (
	"runtime"
	"strings"
)

// Env changes only the returned child environment; notices never include exporter values.
func Env(parent []string, endpoint, service string, override bool) ([]string, []string) {
	values := append([]string(nil), parent...)
	var notices []string
	for _, setting := range []struct{ trace, generic, value string }{
		{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT", endpoint},
		{"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf"},
	} {
		if override {
			setEnv(&values, setting.trace, setting.value)
			continue
		}
		if value := getEnv(values, setting.trace); value != "" {
			notices = append(notices, setting.trace+" is explicitly configured and preserved; verify it targets Tracelet.")
			continue
		}
		if value := getEnv(values, setting.generic); value != "" {
			notices = append(notices, setting.generic+" supplies the trace setting and is preserved; verify it targets Tracelet.")
			continue
		}
		setEnv(&values, setting.trace, setting.value)
	}
	if getEnv(values, "OTEL_SERVICE_NAME") == "" && !hasResourceService(getEnv(values, "OTEL_RESOURCE_ATTRIBUTES")) {
		setEnv(&values, "OTEL_SERVICE_NAME", service)
	}
	if strings.EqualFold(getEnv(values, "OTEL_SDK_DISABLED"), "true") {
		notices = append(notices, "OTEL_SDK_DISABLED remains enabled; this application may emit no telemetry.")
	}
	if strings.EqualFold(getEnv(values, "OTEL_TRACES_EXPORTER"), "none") {
		notices = append(notices, "OTEL_TRACES_EXPORTER remains none; this application may emit no traces.")
	}
	notices = append(notices, "Tracelet requires application instrumentation; programmatic SDK configuration can override environment settings.")
	return values, notices
}

func hasResourceService(attributes string) bool {
	for _, entry := range strings.Split(attributes, ",") {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.TrimSpace(key) == "service.name" && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func envKeyEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
func getEnv(values []string, key string) string {
	for i := len(values) - 1; i >= 0; i-- {
		name, value, ok := strings.Cut(values[i], "=")
		if ok && envKeyEqual(name, key) {
			return value
		}
	}
	return ""
}
func setEnv(values *[]string, key, value string) {
	for i := len(*values) - 1; i >= 0; i-- {
		name, _, ok := strings.Cut((*values)[i], "=")
		if ok && envKeyEqual(name, key) {
			(*values)[i] = key + "=" + value
			return
		}
	}
	*values = append(*values, key+"="+value)
}
