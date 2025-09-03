---
title: "Telemetry & Privacy"
weight: 25
# bookFlatSection: false
# bookToc: true
# bookHidden: false
# bookCollapseSection: false
# bookComments: false
# bookSearchExclude: false
---

# Telemetry & Privacy

Tyk AI Studio collects anonymized usage statistics to help improve the product and understand how features are being used.

## What Data is Collected

The telemetry system collects only aggregate statistics:

- **User counts** by type (admin, developer, chat users)
- **LLM model counts** and token usage
- **Application counts** and proxy usage
- **Chat counts** and interaction statistics
- **User group counts**

**No personal data, content, or sensitive information is collected.**

## Privacy & Security

- All data is **anonymized** before transmission
- **No personally identifiable information (PII)** is collected
- **No chat content, prompts, or responses** are transmitted
- **No API keys or credentials** are included
- Instance identifiers are hashed and rotated daily
- Data is sent over HTTPS to `https://telemetry.tyk.technology`

## Disabling Telemetry

Telemetry is **enabled by default** but can be easily disabled.

### Environment Variable

```bash
export TELEMETRY_ENABLED=false
```

### Configuration File

Add to your `.env` file:
```
TELEMETRY_ENABLED=false
```

### Docker Deployment

```bash
docker run -e TELEMETRY_ENABLED=false your-image
```

### Helm/Kubernetes

In your `values.yaml`:
```yaml
env:
  TELEMETRY_ENABLED: "false"
```

## Verification

When telemetry is disabled, you'll see this log message at startup:
```
Telemetry is disabled
```

When enabled, you'll see:
```
Telemetry collection started - collecting usage statistics every 1h0m0s
Telemetry data will be sent to: https://telemetry.tyk.technology
To disable telemetry, set environment variable: TELEMETRY_ENABLED=false
```

## Technical Details

- Telemetry data is collected every **1 hour**
- Data transmission uses a **30-second timeout**
- Failed telemetry requests **never affect** application performance
- All telemetry errors are logged as warnings only
