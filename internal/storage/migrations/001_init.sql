CREATE TABLE metadata (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    project_id TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE resources (
    resource_id TEXT PRIMARY KEY,
    payload BLOB NOT NULL
);
CREATE TABLE scopes (
    scope_id TEXT PRIMARY KEY,
    payload BLOB NOT NULL
);
CREATE TABLE traces (
    trace_id TEXT PRIMARY KEY,
    start_ns INTEGER NOT NULL,
    end_ns INTEGER NOT NULL,
    span_count INTEGER NOT NULL,
    error_count INTEGER NOT NULL,
    first_seen_ns INTEGER NOT NULL,
    last_seen_ns INTEGER NOT NULL,
    revision INTEGER NOT NULL
);
CREATE TABLE spans (
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT NOT NULL,
    name TEXT NOT NULL,
    service_name TEXT NOT NULL,
    start_ns INTEGER NOT NULL,
    end_ns INTEGER NOT NULL,
    kind INTEGER NOT NULL,
    status INTEGER NOT NULL,
    resource_id TEXT NOT NULL REFERENCES resources(resource_id),
    scope_id TEXT NOT NULL REFERENCES scopes(scope_id),
    payload BLOB NOT NULL,
    PRIMARY KEY (trace_id, span_id),
    FOREIGN KEY (trace_id) REFERENCES traces(trace_id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
CREATE INDEX spans_parent ON spans(trace_id, parent_span_id);
CREATE INDEX spans_start ON spans(trace_id, start_ns, span_id);
CREATE INDEX spans_service ON spans(service_name, trace_id);
CREATE INDEX spans_operation ON spans(name, trace_id);
CREATE INDEX traces_start ON traces(start_ns, trace_id);
CREATE INDEX traces_activity ON traces(last_seen_ns, trace_id);
