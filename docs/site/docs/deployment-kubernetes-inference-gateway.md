---
title: "Running Alongside the Kubernetes Inference Gateway"
weight: 4
---

# Running Alongside the Kubernetes Inference Gateway

Tyk AI Studio and the [Gateway API Inference Extension](https://gateway-api-inference-extension.sigs.k8s.io/)
solve adjacent problems, and they compose without either side needing to change.

The Inference Extension is about **which GPU pod should serve this request**. Its
`InferencePool` selects Pods, and its Endpoint Picker scrapes model-server metrics
(`vllm:num_requests_waiting`, `vllm:kv_cache_usage_perc`, `vllm:lora_requests_info`)
to route on KV-cache affinity, LoRA adapter affinity and queue depth. Every one of
its design decisions assumes you own the accelerators.

The Tyk Microgateway is about **whether this request should happen at all, and
what it costs**: authentication, app-to-model access control, budget enforcement,
content filters, and token and spend accounting — across both self-hosted models
and the SaaS providers (OpenAI, Anthropic, Bedrock, Vertex, Google AI) that most
deployments actually use.

Put together, each does the part it is good at:

```
  client
    │
    ▼
  Gateway (Gateway API)  ──►  HTTPRoute
                                │
                                ▼
                          Tyk Microgateway          identity, budgets, filters,
                                │                   token + cost analytics
                                ▼
                          Service fronting an       endpoint selection by
                          InferencePool             KV-cache / queue depth
                                │
                                ▼
                          vLLM / SGLang / TGI pods
```

Tyk does not implement `InferencePool` and does not act as an Endpoint Picker.
It does not need to: the pool fronts an OpenAI-compatible endpoint, and the
gateway speaks OpenAI-compatible upstream.

## Configuring it

### 1. The LLM configuration in the Studio

Create an LLM pointing at the Service in front of your `InferencePool`:

| Field | Value |
|---|---|
| Vendor | `openai` (the pool serves the OpenAI-compatible API) |
| API Endpoint | `http://vllm-llama3-8b-instruct.inference.svc.cluster.local:8000/v1` |
| API Key | whatever the model server expects, or a placeholder if it needs none |
| Allowed Models | the model names the pool serves, e.g. `meta-llama/Llama-3.1-8B-Instruct` |

Everything else — budgets, filters, privacy scores, app access — behaves exactly
as it does for a hosted provider. The pool is just an upstream.

### 2. Egress policy

By default the gateway will reach an in-cluster address without complaint;
internal-range blocking is opt-in. If you turn it on to constrain egress, the
InferencePool Service becomes unreachable too, because it lives on a private
address.

The exemption is `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS`:

```yaml
config:
  blockInternalUpstreams: true            # LLM_UPSTREAM_BLOCK_INTERNAL
  allowedInternalHosts: ".svc.cluster.local"
```

This is deliberately a separate variable from `LLM_UPSTREAM_ALLOWED_HOSTS`. That
one is a *global* allowlist: using it to permit a cluster host would silently
refuse every external provider not also listed. `LLM_UPSTREAM_ALLOWED_INTERNAL_HOSTS`
is additive — it widens the policy only for the hosts named, so an in-cluster
pool and a SaaS provider can coexist with blocking left on.

Entries match the way the existing allowlist does: `.svc.cluster.local` matches
`vllm.inference.svc.cluster.local` and the apex, but never `evilsvc.cluster.local`.

The exemption is applied both when the upstream URL is validated and at dial
time. It does not reopen the DNS-rebinding protection: an exempted name is one
you declared may resolve internally, not one an attacker chose.

### 3. Routing traffic to the gateway

Point an `HTTPRoute` at the microgateway Service like any other backend:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: ai-gateway
spec:
  parentRefs:
    - name: inference-gateway
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /v1
      backendRefs:
        - name: tyk-microgateway
          port: 8080
```

Callers then use the gateway's own OpenAI-compatible surface — `/v1/chat/completions`
with a `vendor/model` model string, or `/ai/{llm-slug}/v1/chat/completions` — and
authenticate with an App credential, so budgets and filters apply.

### 4. Deploying

`helm/my-values-inference-gateway.yaml` is a working reference for this shape:
scaled out, spread across zones, egress-hardened with the cluster carved out,
metrics scraped by Prometheus and traces exported to OTLP.

```bash
helm upgrade --install ai-studio ./helm -f ./helm/my-values-inference-gateway.yaml
```

## Observability

Both halves of the stack emit OpenTelemetry GenAI metrics, so a single dashboard
covers the whole path. The gateway exposes, per provider and model:

| Metric | Meaning |
|---|---|
| `gen_ai_server_request_duration_seconds` | end-to-end request latency |
| `gen_ai_server_time_to_first_token_seconds` | streaming responsiveness |
| `gen_ai_server_time_per_output_token_seconds` | streaming throughput |
| `gen_ai_client_token_usage` | input and output token counts |
| `gen_ai_client_operation_duration_seconds` | tool execution |

Attributes follow the conventions: `gen_ai_provider_name`, `gen_ai_request_model`,
`gen_ai_operation_name`, `gen_ai_token_type`, and `error_type` on failures.

Cost, policy blocks and compliance events have no counterpart in the conventions
and stay under their `aistudio_*` names.

> The OpenTelemetry GenAI conventions are still marked *Development* upstream,
> not Stable, so these names may shift. The pre-conventions `aistudio_*` series
> is emitted alongside them by default; set `METRICS_LEGACY_NAMES=false`
> (`metrics.legacyNames` in the chart) once your dashboards have migrated.

Set `ENABLE_TRACING` and `TRACING_ENDPOINT` to export spans. The gateway joins an
inbound `traceparent` rather than starting a new trace, and propagates it to the
upstream, so a request stays on one trace from the Gateway API listener through
Tyk to the model server.

## What Tyk deliberately does not do

Tyk is **not** a conformant Gateway API Inference Extension implementation, and
there is no plan for it to become one:

- It does not watch `InferencePool` resources or write their `status.parents[]`.
- It does not implement the Endpoint Picker ext-proc protocol.
- It does not select individual model-server pods.

Doing so would require being a full Gateway API implementation first — the
`InferencePool` status contract is keyed by Gateway, so it entails GatewayClass,
listeners, TLS and route attachment — and would put a second, in-cluster
configuration source next to the hub-spoke stream that the Studio owns.
Composition gets the same outcome at a fraction of the cost.

If you want pod-level inference routing, run one of the conformant
implementations (agentgateway, Higress, NGINX Gateway Fabric, Istio) and put Tyk
in front of it, as above.

## See also

- [Kubernetes / Helm Deployment](deployment-helm-k8s.md)
- [Edge Gateways](edge-gateways.md)
- [LLM Proxy](proxy.md)
