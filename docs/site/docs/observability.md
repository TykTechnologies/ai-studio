---
title: "Observability"
weight: 8
---

# Observability

The AI Gateway exports Prometheus metrics following the
[OpenTelemetry GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
and OpenTelemetry distributed traces. Both work identically in the embedded
gateway inside AI Studio and in the standalone Microgateway, because they share
one proxy implementation.

> This is about the **gateway data path**. Anonymised product usage statistics
> are a separate system — see [Telemetry](./telemetry.md) — and per-app cost
> reporting in the dashboard comes from [Analytics](./analytics.md), which reads
> the database rather than these metrics.

## Enabling the metrics endpoint

`/metrics` is **secure by default**. Unless you set an auth token or explicitly
allow unauthenticated access, the route is not registered at all — a Prometheus
target pointed at it will 404 indefinitely.

```bash
ENABLE_METRICS=true
METRICS_PATH=/metrics

# Pick one:
METRICS_ALLOW_UNAUTHENTICATED=true    # in-cluster scraping
METRICS_AUTH_TOKEN=<token>            # requires "Authorization: Bearer <token>"
```

In Helm, this is `microgateway.metrics.*`. The chart will refuse to render a
ServiceMonitor unless one of the two is configured, rather than shipping a
monitor that silently scrapes nothing:

```yaml
microgateway:
  metrics:
    enabled: true
    allowUnauthenticated: true
    serviceMonitor:
      enabled: true
      interval: 30s
```

To scrape with a token instead, leave `allowUnauthenticated: false` and point
`bearerTokenSecret` at a Secret holding a value matching `METRICS_AUTH_TOKEN`:

```yaml
microgateway:
  metrics:
    bearerTokenSecret:
      name: microgateway-metrics
      key: token
```

## What is exported

### GenAI metrics

Labelled with `gen_ai_provider_name`, `gen_ai_request_model`,
`gen_ai_operation_name`, `gen_ai_token_type`, and `error_type` on failures only.

| Metric | Description |
|---|---|
| `gen_ai_server_request_duration_seconds` | End-to-end request latency |
| `gen_ai_server_time_to_first_token_seconds` | Streaming responsiveness |
| `gen_ai_server_time_per_output_token_seconds` | Streaming throughput |
| `gen_ai_client_token_usage` | Input and output tokens per request |
| `gen_ai_client_operation_duration_seconds` | Tool execution duration |

The two streaming metrics are emitted for streaming requests only — a
non-streaming request records duration and tokens but no first-token or
per-token latency.

Vendors are reported under their registered convention values, so dashboards
built against other gateways line up:

| Configured vendor | Reported as |
|---|---|
| `bedrock` | `aws.bedrock` |
| `vertex` | `gcp.vertex_ai` |
| `google_ai` | `gcp.gemini` |
| `openai`, `anthropic`, `ollama`, `huggingface` | unchanged |

Token types follow the conventions' `input` / `output`. Prompt-cache reads and
writes have no convention value and appear as `cache_read` / `cache_write`.

### Governance metrics

These describe policy rather than inference, have no counterpart in the
conventions, and keep their own names:

| Metric | Labels | Description |
|---|---|---|
| `aistudio_llm_requests_total` | `app_id`, `vendor`, `model`, `status_code` | Proxied requests |
| `aistudio_llm_cost_total` | `vendor`, `model`, `app_id` | Cumulative cost |
| `aistudio_llm_inflight_requests` | `vendor` | Requests in flight |
| `aistudio_policy_blocks_total` | `rule_name`, `block_type` | Blocked by budget, firewall or filter |
| `aistudio_compliance_events_total` | `event_type`, `severity`, `filter_name` | Raised by filter scripts |
| `aistudio_tool_calls_total` | `tool_name`, `app_id` | Tool / MCP invocations |

### Migrating existing dashboards

Three metrics were renamed to their convention equivalents. The old names are
still emitted by default, so nothing breaks on upgrade:

| Old | New |
|---|---|
| `aistudio_llm_request_duration_seconds` | `gen_ai_server_request_duration_seconds` |
| `aistudio_llm_tokens_total` | `gen_ai_client_token_usage` |
| `aistudio_tool_execution_duration_seconds` | `gen_ai_client_operation_duration_seconds` |

Note the token metric changed shape as well as name: the conventions define it as
a histogram, so use `gen_ai_client_token_usage_sum` where you previously used the
counter. Once your dashboards are migrated, set `METRICS_LEGACY_NAMES=false`
(`microgateway.metrics.legacyNames: false`) to stop emitting the old series.

> The GenAI conventions are still marked *Development* upstream, not Stable.
> These names may shift in future releases; that is why the legacy series is kept
> available rather than removed outright.

## Example queries

```promql
# P95 latency by model
histogram_quantile(0.95,
  sum by (le, gen_ai_request_model) (rate(gen_ai_server_request_duration_seconds_bucket[5m])))

# P95 time to first token — the number users actually feel
histogram_quantile(0.95,
  sum by (le) (rate(gen_ai_server_time_to_first_token_seconds_bucket[5m])))

# Output tokens per second by provider
sum by (gen_ai_provider_name) (
  rate(gen_ai_client_token_usage_sum{gen_ai_token_type="output"}[5m]))

# Error rate
sum(rate(gen_ai_server_request_duration_seconds_count{error_type!=""}[5m]))
  / sum(rate(gen_ai_server_request_duration_seconds_count[5m]))

# Spend per hour by app
sum by (app_id) (rate(aistudio_llm_cost_total[1h]))

# Share of requests refused by policy
sum(rate(aistudio_policy_blocks_total[5m])) / sum(rate(aistudio_llm_requests_total[5m]))
```

## Tracing

Span export is disabled by default. To enable it:

```bash
ENABLE_TRACING=true
TRACING_ENDPOINT=otel-collector.observability:4317
```

A bare `host:port` is dialled without TLS; give an `https://` URL to use TLS.
In Helm:

```yaml
microgateway:
  tracing:
    enabled: true
    endpoint: "otel-collector.observability:4317"
```

Each proxy hop produces one span named `{operation} {model}` (for example
`chat gpt-4o`), carrying the same `gen_ai.*` attributes as the metrics, with the
status set to error and `http.response.status_code` recorded on failures.

### Trace propagation

The gateway **joins the caller's trace** rather than starting a new one: an
inbound `traceparent` is extracted and the upstream request carries it onward. A
request therefore stays on a single trace from your ingress, through Tyk, to the
model server.

This works **whether or not tracing is enabled**. The W3C propagator is installed
unconditionally, so enabling tracing changes whether the gateway *records* spans,
not whether it *propagates* them. If you already trace either side of the
gateway, you get an unbroken trace without turning anything on here.

## Kubernetes

The Helm chart ships the pieces needed for a standard monitoring stack:

```yaml
microgateway:
  metrics:
    enabled: true
    allowUnauthenticated: true
    serviceMonitor:
      enabled: true
      interval: 30s
      labels:
        release: kube-prometheus-stack   # match your Prometheus selector
  tracing:
    enabled: true
    endpoint: "otel-collector.observability:4317"
```

When running more than one replica, each pod registers with the control plane
under its own edge identity derived from the pod name, so metrics and analytics
are attributed per replica. See
[Kubernetes / Helm Deployment](./deployment-helm-k8s.md).

## See also

- [Kubernetes / Helm Deployment](./deployment-helm-k8s.md)
- [Running Alongside the Kubernetes Inference Gateway](./deployment-kubernetes-inference-gateway.md)
- [Analytics](./analytics.md) — dashboard reporting, sourced from the database
- [Telemetry](./telemetry.md) — anonymised product usage statistics
