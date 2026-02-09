---
title: "Edge Gateways"
weight: 7
---

# Edge Gateways

> **Note:** Edge Gateway management is an **Enterprise Edition** feature for hub-and-spoke deployments.

Edge Gateways are distributed Microgateway instances that connect to the Tyk AI Studio control plane. They process AI requests locally while receiving configuration from the central hub.

## Overview

In enterprise deployments, Tyk AI Studio supports a hub-and-spoke architecture:

- **Hub (Control Plane):** Central Tyk AI Studio instance managing configuration, policies, and analytics
- **Spoke (Edge Gateways):** Distributed Microgateway instances processing requests locally

This architecture enables:

- **Regional Compliance:** Keep data processing within specific geographic regions
- **Reduced Latency:** Route requests to the nearest edge instance
- **High Availability:** Continue processing even if the hub is temporarily unreachable
- **Multi-Tenancy:** Isolate resources between different teams or customers using namespaces

## Edge Gateway Management UI

The Edge Gateways page in the Admin UI provides visibility into all connected edge instances. Access it via **Admin > Edge Gateways** in the sidebar.

### Edge Gateway List

The list view displays all registered edge gateways with key status information:

| Column | Description |
|--------|-------------|
| **Edge ID** | Unique identifier for the edge gateway |
| **Namespace** | The namespace the edge belongs to (Enterprise) |
| **Connection** | Connection status based on heartbeat: Connected, Disconnected, or Stale |
| **Config Sync** | Whether the edge has the latest configuration |
| **Version** | Software version and build hash of the edge gateway |
| **Last Heartbeat** | Time since the last heartbeat was received |

### Edge Gateway Detail View

Click on any edge gateway to view detailed information:

- **Basic Information:** Edge ID, namespace, status, and session ID
- **Version Information:** Software version, build hash, and last heartbeat timestamp
- **Configuration Sync Status:** Detailed sync status including checksums and last sync acknowledgment
- **Timestamps:** When the edge was registered and last updated
- **Metadata:** Any custom metadata reported by the edge

## Configuration Synchronization

Tyk AI Studio uses a checksum-based system to track configuration synchronization between the control plane and edge gateways.

### How It Works

1. **Checksum Generation:** When configuration changes occur on the control plane, a SHA-256 checksum is computed from the serialized configuration snapshot
2. **Heartbeat Reporting:** Edge gateways report their loaded configuration checksum in each heartbeat
3. **Status Comparison:** The control plane compares reported checksums to determine sync status
4. **UI Notifications:** The admin UI displays sync status and notifies administrators when edges are out of sync

### Configuration Objects in Checksum

The configuration checksum includes objects that need to be synchronized to edge gateways for request processing:

- **LLM Configurations** - AI provider settings and credentials
- **Filters** - Request/response processing rules
- **Plugins** - Gateway plugin configurations
- **Model Prices** - Cost tracking configurations
- **Model Routers** - Request routing rules (Enterprise)

Any create, update, or delete operation on these objects triggers a checksum recalculation.

### Apps and Credentials

**Apps** are synced as part of the configuration snapshot but are **not** included in the checksum calculation. This is because Apps change frequently (users create and update them regularly), and including them in the checksum would cause unnecessary sync churn.

**Credentials** (access tokens) are **not** pulled during the initial configuration snapshot. Instead, Microgateways use a **pull-on-miss** caching strategy:

1. When a gateway receives a request with an unknown access token, it contacts AI Studio to validate and fetch the credential.
2. The credential is then cached locally for subsequent requests.
3. This ensures the admin retains ongoing control — disabling a credential in AI Studio takes effect as soon as the gateway's cache expires or the next pull-on-miss occurs.

This approach balances performance (no need to sync every credential change) with security (admin can revoke access without waiting for a full config push).

> **Note:** Tools and Datasources are used by AI Studio's chat functionality and RAG system respectively. They are not proxied by edge gateways and are not part of the edge gateway configuration. See [Architecture Overview](./architecture.md) for details on this design decision.

### Sync Status Values

| Status | Description | UI Indicator |
|--------|-------------|--------------|
| **In Sync** | Edge has the current configuration | Green chip |
| **Pending** | Edge needs a configuration update | Yellow chip |
| **Stale** | Edge has been out of sync for >15 minutes | Orange chip |
| **Unknown** | Edge hasn't reported a checksum yet | Gray chip |

### Sync Status Banner

When any edge gateways are out of sync, a warning banner appears at the top of the admin UI. The banner:

- Shows the number of edges requiring updates
- Provides a direct link to the Edge Gateways page
- Automatically disappears when all edges are synchronized
- Updates immediately after pushing configuration

## Pushing Configuration

Configuration changes are pushed to edge gateways on-demand (not automatically) to ensure administrators maintain control over when changes are deployed.

### Push Configuration Modal

Click the **Push Configuration** button to open the push modal. You can choose to:

1. **Push to All Namespaces:** Sends configuration to all connected edge gateways
2. **Push to Specific Namespace:** Sends configuration only to edges in a selected namespace (Enterprise)

### Push Process

When you push configuration:

1. The control plane generates a new configuration snapshot for the target namespace(s)
2. Edge gateways receive a reload signal via gRPC
3. Each edge fetches the new configuration and applies it
4. Edges report the new checksum in their next heartbeat
5. The sync status updates to reflect the new state

### Monitoring Push Results

After pushing configuration:

- The sync status banner updates within a few seconds
- Individual edge sync status is visible in the list and detail views
- Hover over the Config Sync chip to see checksum details

## Checksum Details

For debugging sync issues, the UI displays checksum information:

- **Loaded Config Checksum:** The checksum reported by the edge gateway
- **Expected Config Checksum:** The checksum expected by the control plane (shown when out of sync)
- **Loaded Config Version:** Version string of the edge's current configuration
- **Last Sync Acknowledgment:** Timestamp when the edge last confirmed receiving a configuration

Hover over the Config Sync status chip in the list view to see a truncated checksum comparison.

## Removing Edge Gateways

To remove an edge gateway entry from the control plane:

1. Click the three-dot menu (⋮) on the edge row, or go to the detail view
2. Select **Remove Entry**
3. Confirm the removal

> **Note:** This removes the entry from the control plane database. If the edge gateway is still running, it will re-register on its next connection attempt.

## API Endpoints

The Edge Gateway sync status can also be queried via the REST API:

### Get Sync Status Summary

```
GET /api/v1/sync/status
```

Returns sync status for all namespaces:

```json
{
  "data": [
    {
      "namespace": "default",
      "expected_checksum": "abc123...",
      "last_config_change": "2024-01-15T10:30:00Z",
      "synced_count": 3,
      "pending_count": 1,
      "stale_count": 0,
      "total_edges": 4
    }
  ],
  "has_pending": true
}
```

### Get Namespace Sync Status

```
GET /api/v1/sync/status/:namespace
```

Returns detailed sync status for a specific namespace, including per-edge status.

### Trigger Configuration Reload

```
POST /api/v1/edges/reload
```

Triggers a configuration reload for all edges or a specific namespace:

```json
{
  "namespace": "production",
  "scope": "namespace"
}
```

## Troubleshooting

### Edge Shows "Disconnected"

- Check network connectivity between the edge and control plane
- Verify the edge gateway is running and healthy
- Check edge gateway logs for connection errors
- Ensure firewall rules allow gRPC traffic (default port 50051)

### Edge Shows "Pending" After Push

- Wait a few seconds for the heartbeat cycle to complete
- Check if the edge is connected (not disconnected)
- Verify the edge gateway logs for configuration load errors
- Check if the edge has sufficient permissions to fetch configuration

### Checksum Mismatch Persists

- Try pushing configuration again
- Check for configuration validation errors in edge logs
- Verify the edge and control plane are running compatible versions
- Check for database replication lag if using PostgreSQL replication

### Sync Status Banner Doesn't Disappear

- Verify all edges have successfully loaded the new configuration
- Check for any disconnected edges that can't receive updates
- Refresh the page to ensure the latest status is displayed
