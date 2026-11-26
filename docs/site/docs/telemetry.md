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

Tyk AI Studio can send anonymized usage statistics to Tyk to help improve the product and understand how features are being used. **Telemetry is off by default (since 2.3)**: nothing is sent unless you opt in.

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

## Enabling Telemetry

There are two independent telemetry reports, both off by default:

| Report | Editions | Turn on with |
|--------|----------|--------------|
| Anonymized usage counts (described above) | Community and Enterprise | `TELEMETRY_ENABLED=true` |
| License usage reports (usage counts and API interactions, license key hashed) | Enterprise | `LICENSE_DISABLE_TELEMETRY=false` |

### Environment Variable or `.env` File

```bash
TELEMETRY_ENABLED=true
LICENSE_DISABLE_TELEMETRY=false   # Enterprise Edition
```

### Docker Compose

The quickstart env files (`quickstart/confs/midsommar-*.env`) set both explicitly to off; change them there.

### Helm/Kubernetes

In your `values.yaml`:
```yaml
config:
  telemetryEnabled: true
  licenseTelemetryEnabled: true   # Enterprise Edition
```

### Linux Packages (deb/rpm)

Edit `/etc/default/tyk-ai-studio` (also `/etc/sysconfig/tyk-ai-studio` on RPM systems), which a new install writes with both reports off, then `systemctl restart tyk-ai-studio`.

### Upgrading from Earlier Versions

Before 2.3 both reports were on unless turned off. On upgrade they turn off unless your configuration sets `TELEMETRY_ENABLED=true` or `LICENSE_DISABLE_TELEMETRY=false`; an existing `TELEMETRY_ENABLED=false` or `LICENSE_DISABLE_TELEMETRY=true` keeps working.

## Verification

When usage telemetry is off, you'll see this log message at startup:
```
Telemetry is disabled
```

When it is on, you'll see:
```
Telemetry collection started - collecting usage statistics every 1h0m0s
Telemetry data will be sent to: https://telemetry.tyk.technology
Telemetry is on because TELEMETRY_ENABLED=true; unset it or set it to false to turn it off
```

## Technical Details

- Telemetry data is collected every **1 hour**
- Data transmission uses a **30-second timeout**
- Failed telemetry requests **never affect** application performance
- All telemetry errors are logged as warnings only
