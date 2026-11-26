# Feasibility spike

T02 isolates the first interoperability and storage experiments from the production CLI. It uses synthetic data only. Passing the spike does not make the receiver safe for arbitrary applications: validation, redaction, overload handling, migration, retention, session ownership and security belong to T03–T12.

## Questions and evidence

The spike must demonstrate a real Go SDK exporter using HTTP/Protobuf and gzip, a committed SQLite write before acknowledgement, the original trace identity and metadata after close/reopen, and a final SDK shutdown flush. It also compares indexed span records with a Protobuf detail payload against normalized attributes using the same generated workload. Commands and raw measurements are recorded in the experiment README and `docs/benchmarks.md` after execution.

The initial library candidates are Go 1.27.4, OpenTelemetry Go SDK/exporter v1.47.0, generated OTLP v1.11.1 and modernc SQLite v1.63.0 (with its required libc v1.80.1). The SDK and generated protocol modules are Apache-2.0; the SQLite driver is BSD-3-Clause. Dependencies remain development/runtime implementation choices, not authorization for the project's own license.

## Result and decision

The real gzip exporter check, fresh-process reopen assertion, race detector and vet pass. The known trace ID, parent IDs, resource/scope metadata, custom source attributes, event/link/array fields and exact timestamps/int64 values survive. A deliberately long SDK batch timer verifies that explicit SDK shutdown performs the final export. Custom spans supply source metadata explicitly; automatic source coverage has not been established.

The 100,000-span comparison supports indexed scalar columns plus bounded Protobuf details as the initial production layout. See [benchmark evidence](benchmarks.md). Use one writer and WAL with synchronous FULL as the starting durability policy, applying connection-specific settings to every connection; validate contention, checkpoint behavior, migrations and crash recovery in production tasks. Preserve resources/scopes separately where useful and do not build arbitrary-attribute indexes without a measured query requirement.

Proceed with the in-process generated-Protobuf receiver and modernc SQLite. An explicit SDK `WithEndpointURL` requires the complete `/v1/traces` URL; the spike caught and corrected a base-URL assumption. Generic OTEL environment base URLs have separate suffix rules, so T08 must exercise that configuration matrix directly.

## Developer interview script

Invite 5–10 backend developers only when outreach is authorized. Record role/runtime and consent; do not collect private production traces. For each participant:

1. Ask for a recent slow or failing local request and the steps used to investigate it.
2. Observe a synthetic reproduction with their existing tools. Record elapsed time, tools opened and the point where causality becomes unclear.
3. Ask which spans their current instrumentation emits and whether file/function/line attributes are available.
4. Show the representative trace prototype. Ask the participant to identify the slowest operation, inspect its error/attributes and find its source location without guidance.
5. Ask what they would install, what local data they would allow stored, and what prevents use in their actual workflow.
6. Record task success and misunderstandings. Distinguish observed behavior from the participant's predictions or our hypotheses.

Do not claim interviews or usability success until actual sessions occur. Proposed targets remain under five minutes to first trace after installation and at least 80% of participants locating the slowest span unassisted.

## Competitive workflow worksheet

Compare the same synthetic slow/error workload in Jaeger, Grafana Tempo, SigNoz and the developer's IDE/log workflow. For each, record the version, installation commands, required services/accounts, offline operation, time to first trace, hierarchy/error inspection, source metadata/navigation, export, local privacy defaults and cleanup. Use official documentation and direct exercise at the time of comparison. No evaluation of these products has been performed in this spike; the differentiation claim remains a hypothesis.

## Clickable prototype brief

The O05 prototype should use the same representative synthetic trace shape: HTTP root, nested custom spans, one slow operation, one error, an event and source metadata. Include a trace list, expandable waterfall, keyboard-selectable span panel and copyable source location. Include missing telemetry, orphan spans, no source metadata and redacted attributes. Label the data as synthetic. Test the investigation tasks before visual polish; reuse successful interactions in T13–T16 rather than maintaining a second UI implementation.
