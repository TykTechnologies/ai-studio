# Plugin SDK Changes in v2.1

This page is a focused reference for what changed in the **Unified Plugin SDK** (`pkg/plugin_sdk`) and the **AI Studio SDK** (`pkg/ai_studio_sdk`) in **v2.1.0**. It exists so plugin authors can scan a short list and decide what (if anything) needs to change in their plugins, without having to diff the full SDK package.

For the complete SDK reference, see [Plugin SDK Reference](plugins-sdk.md) and [Service API Reference](plugins-service-api.md).

---

## Summary

| Change | Where | Impact |
|---|---|---|
| `StoreApp` on `GatewayServices` | Gateway plugins | New capability — opt-in. Requires `apps.write` scope. |
| `ListModelPrices` / `GetModelPricesByVendor` on `ai_studio_sdk` | Studio plugins | New capability — opt-in. Requires `pricing.read` scope. |
| UTF-8 sanitization at the gRPC boundary | All plugins | Transparent — `SanitizeUTF8` / `SanitizeUTF8String` available if you build proto strings from arbitrary `[]byte`. |
| `AnalyticsHandler.RecordComplianceEvents` (new) | Custom analytics handlers | Additive interface method — implement if you provide a custom handler. |
| `AnalyticsHandler` context propagation | Custom analytics handlers | Signature change — recording methods now take `context.Context`. |
| `ComplianceEvents` on `ScriptOutput` | Filter scripts / script runners | Filter scripts can attach compliance events to their output. See [Compliance Events](compliance-events.md). |

None of the changes require existing plugins to be modified unless you are implementing one of the affected interfaces (`AnalyticsHandler`).

---

## New: `StoreApp` on `GatewayServices`

Gateway plugins can now write Apps directly into the local Microgateway SQLite database. This enables auth plugins to provision apps dynamically from the control plane without waiting for the periodic config sync.

The canonical use case is the OAuth2 client-credentials enterprise plugin: when a previously unseen client authenticates against an upstream IdP, the gateway publishes an event up to Studio, Studio creates the app from a template and publishes the full `AppConfig` back down, and the gateway plugin calls `StoreApp` to persist it locally for immediate use.

### Interface

```go
// Defined in pkg/plugin_sdk/context.go (GatewayServices interface)
StoreApp(ctx context.Context, appID uint32, name, description string, isActive bool,
    userID uint32, metadata string, namespace string,
    llmIDs, toolIDs, datasourceIDs []uint32) error
```

### Behavior

- **Upsert semantics**: creates the app if it doesn't exist, updates it if it does. Keyed by `appID`.
- **Transactional associations**: when updating, the existing rows in `app_tools` and `app_datasources` are cleared and replaced atomically. If clearing fails, the transaction rolls back — you won't end up with stale associations alongside new ones.
- **Required scope**: `apps.write` in the plugin manifest.
- **Runtime**: gateway only. The studio stub returns an error so multi-runtime plugins should gate on `ctx.Runtime`.

### Example

```go
func (p *MyAuthPlugin) provisionApp(ctx plugin_sdk.Context, appConfig AppConfig) error {
    if ctx.Runtime != plugin_sdk.RuntimeGateway {
        return nil
    }

    return ctx.Services.Gateway().StoreApp(
        ctx,
        appConfig.AppID,
        appConfig.Name,
        appConfig.Description,
        true,                 // isActive
        appConfig.UserID,
        appConfig.MetadataJSON,
        appConfig.Namespace,
        appConfig.LLMIDs,
        appConfig.ToolIDs,
        appConfig.DatasourceIDs,
    )
}
```

### Manifest

```json
{
  "scopes": [
    "apps.write"
  ]
}
```

See [Plugins → Manifests & Permissions](plugins-manifests.md) for the full scope list.

---

## New: `ListModelPrices` and `GetModelPricesByVendor`

The `ai_studio_sdk` package now exposes the two missing wrappers around the model-prices gRPC service. Studio plugins (the `llm-price-sync` community plugin is the reference consumer) can list or look up model pricing from the control plane.

### Signatures

```go
// pkg/ai_studio_sdk/service_api.go

// ListModelPrices lists model prices with optional vendor filter and pagination.
func ListModelPrices(ctx context.Context, vendor string, page, limit int32) (*mgmtpb.ListModelPricesResponse, error)

// GetModelPricesByVendor retrieves all model prices for a specific vendor.
func GetModelPricesByVendor(ctx context.Context, vendor string) (*mgmtpb.GetModelPricesByVendorResponse, error)
```

Both require the `pricing.read` scope.

### Example: list all prices

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/ai_studio_sdk"

resp, err := ai_studio_sdk.ListModelPrices(ctx, "", 1, 100)
if err != nil {
    return fmt.Errorf("list model prices: %w", err)
}

for _, p := range resp.ModelPrices {
    log.Printf("%s/%s: input=$%.6f/1K output=$%.6f/1K",
        p.Vendor, p.Model, p.InputPricePer1K, p.OutputPricePer1K)
}
```

### Example: filter by vendor

```go
resp, err := ai_studio_sdk.GetModelPricesByVendor(ctx, "anthropic")
if err != nil {
    return err
}
for _, p := range resp.ModelPrices {
    // ...
}
```

The Gateway side already had `GetModelPrice(modelName, vendor)` and `ListModelPrices(vendor)` on `ctx.Services.Gateway()` for the local SQLite read path. The new Studio SDK wrappers complete the picture from the control plane side.

---

## New: UTF-8 sanitization helpers

Protobuf 3 string fields require valid UTF-8. Plugins that read arbitrary file content, scrape web pages, or wrap third-party RPC responses can end up holding `[]byte` data that is not valid UTF-8 (Latin-1 files are the most common culprit). If you cast that data straight into a proto string field, the gRPC marshaller returns `string field contains invalid UTF-8` and the call fails.

v2.1 adds two helpers and applies them at the SDK's own gRPC boundary so plugins don't have to think about this for SDK-mediated calls.

### Helpers

```go
// pkg/plugin_sdk/utf8.go

// SanitizeUTF8 ensures a byte slice produces a valid UTF-8 string by replacing
// invalid sequences with the Unicode replacement character (U+FFFD).
func SanitizeUTF8(b []byte) string

// SanitizeUTF8String ensures a string is valid UTF-8 by replacing invalid
// sequences with the Unicode replacement character (U+FFFD).
func SanitizeUTF8String(s string) string
```

Both are zero-allocation when the input is already valid UTF-8.

### Where it's applied automatically

- `Call()` and `PortalCall()` in the plugin SDK wrap the underlying `[]byte` RPC response before casting it to a string. Any plugin that returns binary data through these paths is now safe by default.
- The community RAG simple chunker's `getOverlap()` was a separate fix in the community submodule — it now advances the slice boundary to the next valid UTF-8 character start to avoid orphaning continuation bytes when chunking emoji or other multi-byte content.

### Where to apply it yourself

If your plugin builds proto string fields from external data (file contents, third-party API responses, user input that bypasses normal validation), wrap the conversion:

```go
import "github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"

content, _ := os.ReadFile(path)
resp := &pb.ProcessFileResponse{
    Content: plugin_sdk.SanitizeUTF8(content),
}
```

Or for strings already in hand:

```go
sanitized := plugin_sdk.SanitizeUTF8String(rawString)
```

---

## Changed: `AnalyticsHandler` interface

The analytics handler interface in `services/analytics` now:

1. Has a new method `RecordComplianceEvents` for the compliance-events pipeline (see [Compliance Events](compliance-events.md)).
2. Takes `context.Context` as the first argument on every recording method.

### Before (v2.0)

```go
type AnalyticsHandler interface {
    RecordProxyLog(log *models.ProxyLog)
    RecordChatLog(entry *models.LLMChatLogEntry)
    RecordContentMessage(...)
    // ... no compliance events
}
```

### After (v2.1)

```go
type AnalyticsHandler interface {
    RecordProxyLog(ctx context.Context, log *models.ProxyLog)
    RecordChatLog(ctx context.Context, entry *models.LLMChatLogEntry)
    RecordContentMessage(ctx context.Context, ...)
    RecordComplianceEvents(ctx context.Context, events []*models.ComplianceEvent)
}
```

### Why

Context propagation lets OTEL trace IDs survive into the analytics path, so the new `/metrics` histograms and counters (`aistudio_llm_request_duration_seconds`, `aistudio_compliance_events_total`, etc.) can correlate with traces. Async goroutines that previously called these methods now use `context.WithoutCancel(r.Context())` to preserve the trace context without coupling the analytics record to the HTTP request lifecycle.

### Migration

If you have a custom `AnalyticsHandler` implementation:

1. Update method signatures to accept `context.Context` as the first argument.
2. Implement `RecordComplianceEvents`. If you don't care about compliance events, the body can be a no-op:
   ```go
   func (h *MyHandler) RecordComplianceEvents(_ context.Context, _ []*models.ComplianceEvent) {}
   ```

The Microgateway's built-in handler (`MicrogatewaAnalyticsHandler`) implements `RecordComplianceEvents` by forwarding events to the pulse plugin for batch transmission to the control plane — see [Compliance Events → Edge gateways](compliance-events.md#edge-gateways-microgateway) for the propagation path.

---

## New: `ComplianceEvents` on `ScriptOutput`

Filter scripts gained a fifth optional output field:

```go
// scripting/types.go

type ScriptOutput struct {
    Block            bool                     // If true, stops the request/response chain
    Payload          string                   // Modified content (empty = no modification)
    Messages         []map[string]interface{} // Modified messages array (alternative to Payload)
    Message          string                   // Optional blocking reason or log message
    ComplianceEvents []ComplianceEventOutput  // Compliance events to record (enterprise only)
}

type ComplianceEventOutput struct {
    EventType   string                 // Free-form type: "pii_redacted", "content_rewritten", ...
    Severity    string                 // "info", "warning", "critical"
    Description string                 // Human-readable description
    Metadata    map[string]interface{} // Arbitrary key-value data
}
```

This is consumed by `scripting.RecordComplianceEvents`, which enriches each event with filter context (`AppID`, `UserID`, `LLMID`, `Vendor`, `ModelName`, `FilterName`, `FilterScope`, `TimeStamp`) and hands them to the analytics pipeline.

If you implement a custom script runner or a Tengo extension that produces `ScriptOutput`, you can populate `ComplianceEvents` directly. The Tengo extraction logic in the enterprise scripting runner reads them from the script output map under the `compliance_events` key.

See [Filters → Compliance Event Reporting](filters.md#compliance-event-reporting) for the Tengo-side syntax and [Compliance Events](compliance-events.md) for the full lifecycle.

---

## Other v2.1 plugin-relevant changes

These aren't SDK API changes, but they affect plugin authors:

- **Plugin reload on config change**: previously, updating a plugin's configuration in Studio left the running plugin process holding stale config. v2.1 reloads plugins when their configuration changes, so `Initialize(ctx, config)` is called again with the new map.
- **Soft-delete conflict on plugin resources fixed**: re-creating a plugin resource with the same identifier as a soft-deleted one no longer conflicts.
- **gRPC datasource secrets**: all gRPC datasource operations (`GenerateEmbedding`, `StoreDocuments`, `ProcessAndStoreDocuments`, `QueryDatasourceByVector`, `DeleteDocumentsByMetadata`, `QueryByMetadataOnly`, `ListNamespaces`, `DeleteNamespace`) now use the new `GetDatasourceByIDResolved` service method which resolves `$SECRET/` references. Plugin code that calls these RPCs gets real credentials instead of placeholder reference strings.
- **LLM `Metadata` secrets**: `$SECRET/` and `$ENV/` references in the LLM `Metadata` map are now resolved through the same code path as `APIKey` and `APIEndpoint`. Plugins reading LLM config (e.g. via `ctx.Services.Gateway().GetLLM(...)` or `ai_studio_sdk.GetLLM(...)`) see resolved values for Bedrock-style credentials stored in `metadata`.

---

## See also

- [Plugin SDK Reference](plugins-sdk.md) — full SDK reference.
- [Service API Reference](plugins-service-api.md) — service interfaces including `GatewayServices`.
- [Manifests & Permissions](plugins-manifests.md) — scope list (`apps.write`, `pricing.read`, etc.).
- [Compliance Events](compliance-events.md) — the broader feature these SDK additions support.
- [Edge-to-Control Communication](plugins-edge-to-control.md) — context for the analytics-pulse path used by edge compliance events.
