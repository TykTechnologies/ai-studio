# Namespaces and Hub-and-Spoke

This guide explains the namespace system in the microgateway and how namespaces enable multi-tenant hub-and-spoke deployments.

## Overview

Namespace features:
- **Multi-Tenant Isolation**: Complete separation between tenants
- **Configuration Filtering**: Selective configuration distribution to edges
- **Access Control**: Fine-grained permission management
- **Resource Isolation**: Separate analytics, budgets, and credentials
- **Backwards Compatibility**: Existing installations work unchanged
- **Flexible Deployment**: Support various multi-tenancy patterns

## Namespace Concepts

### What are Namespaces?
Namespaces provide logical isolation for microgateway resources:
- **Global Namespace** (`""` empty string): Visible to all edges
- **Tenant Namespaces** (`"tenant-1"`): Visible only to matching edges
- **Environment Namespaces** (`"production"`): Environment-specific isolation
- **Team Namespaces** (`"team-a"`): Team-based resource separation

### Namespace Scope
Namespaces apply to all core entities:
- **LLMs**: Provider configurations
- **Applications**: Multi-tenant applications
- **Tokens**: Authentication tokens
- **Model Prices**: Pricing configurations
- **Plugins**: Plugin configurations
- **Filters**: Request/response filters

## Namespace Implementation

### Database Schema
All core tables include a namespace column:
```sql
-- LLM table with namespace
CREATE TABLE llms (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    vendor VARCHAR(50) NOT NULL,
    namespace VARCHAR(255) NOT NULL DEFAULT '',
    -- other fields...
    UNIQUE(name, namespace)
);

-- Application table with namespace
CREATE TABLE apps (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL DEFAULT '',
    -- other fields...
    UNIQUE(name, namespace)
);
```

### Configuration Filtering
Edge instances receive filtered configuration:
```sql
-- Edge with namespace "tenant-a" receives:
SELECT * FROM llms 
WHERE namespace = '' OR namespace = 'tenant-a';

-- Global namespace (empty string) visible to all
-- Specific namespace only visible to matching edges
```

## Namespace Configuration

### Edge Namespace Assignment
```bash
# Assign namespace to edge instance
GATEWAY_MODE=edge
EDGE_NAMESPACE=tenant-a
CONTROL_ENDPOINT=control:50051
EDGE_ID=tenant-a-edge-1
./microgateway

# Edge only receives configuration for:
# - Global namespace ("")
# - Matching namespace ("tenant-a")
```

### Creating Namespaced Resources
```bash
# Create global LLM (visible to all edges)
mgw llm create \
  --name="Global GPT-4" \
  --namespace="" \
  --vendor=openai \
  --model=gpt-4

# Create tenant-specific LLM
mgw llm create \
  --name="Tenant A GPT-4" \
  --namespace="tenant-a" \
  --vendor=openai \
  --model=gpt-4

# Create environment-specific LLM
mgw llm create \
  --name="Production GPT-4" \
  --namespace="production" \
  --vendor=openai \
  --model=gpt-4
```

## Multi-Tenant Deployment Patterns

### Dedicated Edge per Tenant
```bash
# Tenant A dedicated edge
GATEWAY_MODE=edge
EDGE_NAMESPACE=tenant-a
EDGE_ID=tenant-a-edge-1
./microgateway

# Tenant B dedicated edge
GATEWAY_MODE=edge
EDGE_NAMESPACE=tenant-b
EDGE_ID=tenant-b-edge-1
./microgateway

# Each tenant gets isolated edge with only their configuration
```

### Shared Edge with Namespace Isolation
```bash
# Shared edge serving multiple tenants
GATEWAY_MODE=edge
EDGE_NAMESPACE=shared
EDGE_ID=shared-edge-1
./microgateway

# Applications use namespace-specific tokens for access control
# Runtime isolation through application-level authentication
```

### Environment-Based Namespaces
```bash
# Development edge
GATEWAY_MODE=edge
EDGE_NAMESPACE=development
EDGE_ID=dev-edge-1
./microgateway

# Staging edge
GATEWAY_MODE=edge
EDGE_NAMESPACE=staging
EDGE_ID=staging-edge-1
./microgateway

# Production edge
GATEWAY_MODE=edge
EDGE_NAMESPACE=production
EDGE_ID=prod-edge-1
./microgateway
```

## Namespace Management

### Creating Namespaced Configurations
```bash
# Global resources (shared across all namespaces)
mgw llm create --name="Shared GPT-4" --namespace="" --vendor=openai
mgw app create --name="Global Admin" --namespace="" --email=admin@company.com

# Tenant-specific resources
mgw llm create --name="Tenant A LLM" --namespace="tenant-a" --vendor=anthropic
mgw app create --name="Tenant A App" --namespace="tenant-a" --email=tenant-a@company.com

# Environment-specific resources
mgw llm create --name="Dev GPT-4" --namespace="development" --vendor=openai
mgw app create --name="Dev App" --namespace="development" --email=dev@company.com
```

### Namespace Visibility Rules
```
Global Namespace (""):
├── Visible to ALL edges regardless of their namespace
├── Shared LLM configurations
├── Common application templates
└── Global policies and settings

Specific Namespace ("tenant-a"):
├── Only visible to edges with namespace="tenant-a"
├── Tenant-specific LLM configurations
├── Tenant applications and credentials
└── Tenant-specific policies

Edge Receives:
├── All global namespace resources
└── Resources matching edge namespace
```

## Namespace CLI Operations

### Namespace-Aware Commands
```bash
# Create resources with namespace
mgw llm create --namespace="tenant-a" --name="Tenant LLM" --vendor=openai
mgw app create --namespace="tenant-a" --name="Tenant App" --email=user@tenant-a.com

# List resources by namespace
mgw llm list --namespace="tenant-a"
mgw app list --namespace=""  # Global namespace

# List all namespaces
mgw namespace list

# Get namespace information
mgw namespace get tenant-a
```

### Cross-Namespace Operations
```bash
# List resources across all namespaces (admin only)
mgw llm list --all-namespaces

# Move resource between namespaces
mgw llm update 1 --namespace="new-namespace"

# Copy resource to different namespace
mgw llm copy 1 --target-namespace="tenant-b" --new-name="Copied LLM"
```

## Namespace Security

### Access Control
```bash
# Namespace-based access control
# Edges only see resources in their namespace
# Applications can only access resources in their namespace
# Tokens are namespace-scoped
```

### Tenant Isolation
```bash
# Complete isolation between tenants
# No cross-tenant resource access
# Separate analytics and billing
# Independent credential management
# Isolated error logging
```

### Global Resources
```bash
# Global resources are shared but isolated
# Global LLMs available to all tenants
# No cross-tenant data leakage
# Shared costs attributed per tenant usage
```

## Namespace Monitoring

### Per-Namespace Analytics
```bash
# Get analytics for specific namespace
mgw analytics summary 1 --namespace="tenant-a"

# Namespace usage breakdown
mgw analytics events 1 --format=json | \
  jq '.data | group_by(.namespace) | map({namespace: .[0].namespace, count: length})'

# Cost by namespace
mgw analytics costs 1 --format=json | \
  jq '.data.cost_by_namespace'
```

### Cross-Namespace Monitoring
```bash
# Monitor all namespaces (admin view)
for ns in $(mgw namespace list --format=json | jq -r '.data[].name'); do
  echo "Namespace: $ns"
  mgw analytics summary --namespace="$ns" 1
  echo
done
```

## Namespace Use Cases

### SaaS Multi-Tenancy
```bash
# Each customer gets dedicated namespace
mgw app create --namespace="customer-acme" --name="ACME Corp" --email=admin@acme.com
mgw app create --namespace="customer-beta" --name="Beta Inc" --email=admin@beta.com

# Customers only see their own resources
# Complete billing and analytics isolation
```

### Team-Based Separation
```bash
# Development teams
mgw app create --namespace="team-backend" --name="Backend Team" --email=backend@company.com
mgw app create --namespace="team-frontend" --name="Frontend Team" --email=frontend@company.com

# Each team manages their own AI/LLM usage
# Separate budgets and access controls
```

### Environment Isolation
```bash
# Environment-based namespaces
mgw llm create --namespace="development" --name="Dev GPT-4" --vendor=openai
mgw llm create --namespace="staging" --name="Staging GPT-4" --vendor=openai
mgw llm create --namespace="production" --name="Prod GPT-4" --vendor=openai

# Prevent cross-environment access
# Environment-specific configurations
```

### Geographic Separation
```bash
# Region-based namespaces
mgw app create --namespace="us-west" --name="US West App" --email=ops-usw@company.com
mgw app create --namespace="eu-west" --name="EU West App" --email=ops-eu@company.com

# Compliance with data residency requirements
# Regional cost attribution
```

## Namespace Migration

### Adding Namespaces to Existing Deployment
```bash
# 1. Run database migration to add namespace columns
./microgateway -migrate

# 2. Existing resources default to global namespace ("")
# 3. Create new namespaced resources as needed
mgw llm create --namespace="new-tenant" --name="New Tenant LLM"

# 4. Deploy namespace-specific edges
EDGE_NAMESPACE=new-tenant ./microgateway
```

### Migrating Resources Between Namespaces
```bash
# Update resource namespace
mgw llm update 1 --namespace="target-namespace"
mgw app update 1 --namespace="target-namespace"

# Resources become visible to edges with target namespace
# Resources become invisible to edges with previous namespace
```

## Advanced Namespace Features

### Namespace Hierarchies
```bash
# Hierarchical namespace structure (conceptual)
# Global: ""
# ├── Production: "production"
# │   ├── Team A: "production.team-a"
# │   └── Team B: "production.team-b"
# └── Development: "development"
#     ├── Team A: "development.team-a"
#     └── Team B: "development.team-b"

# Implementation uses flat namespace structure
# Hierarchy achieved through naming conventions
```

### Namespace Templates
```yaml
# Namespace configuration templates
namespace_templates:
  tenant_template:
    default_budget: 1000.0
    default_rate_limit: 100
    required_llms: ["gpt-4", "claude-sonnet"]
    security_policy: "standard"
    
  enterprise_template:
    default_budget: 10000.0
    default_rate_limit: 1000
    required_llms: ["gpt-4", "claude-sonnet", "gemini-pro"]
    security_policy: "enhanced"
    compliance_required: true
```

### Cross-Namespace Sharing
```bash
# Shared resources via global namespace
# Global LLMs accessible to all tenants
# Shared costs attributed per tenant usage
# Central policy management

# Example: Shared expensive LLM
mgw llm create \
  --name="Shared GPT-4" \
  --namespace="" \
  --vendor=openai \
  --budget=10000.0  # Shared budget across all tenants
```

## Troubleshooting Namespaces

### Namespace Visibility Issues
```bash
# Check edge namespace configuration
echo $EDGE_NAMESPACE

# Verify resource namespace
mgw llm get 1 --format=json | jq '.data.namespace'

# List resources in specific namespace
mgw llm list --namespace="tenant-a"
```

### Cross-Namespace Access Issues
```bash
# Verify application namespace
mgw app get 1 --format=json | jq '.data.namespace'

# Check LLM associations
mgw app llms 1

# Ensure LLMs and apps are in compatible namespaces
```

### Configuration Propagation Issues
```bash
# Check namespace filtering
grep "namespace filter" /var/log/microgateway/control.log

# Verify edge receives correct configuration
curl http://edge:8080/api/v1/cache/config | jq '.llms'

# Force namespace resync
mgw edge resync edge-1
```

## Best Practices

### Namespace Design
- **Clear Naming Conventions**: Use consistent namespace naming
- **Logical Grouping**: Group related resources in same namespace
- **Access Patterns**: Design namespaces around access patterns
- **Security Boundaries**: Align namespaces with security requirements

### Multi-Tenancy
- **Tenant Isolation**: Each tenant gets dedicated namespace
- **Resource Sharing**: Use global namespace for shared resources
- **Cost Attribution**: Track costs per namespace for billing
- **Security Policies**: Namespace-specific security policies

### Operations
- **Change Management**: Test namespace changes in development first
- **Monitoring**: Monitor namespace usage and performance
- **Backup**: Include namespace information in backups
- **Documentation**: Document namespace usage and conventions

---

Namespaces enable secure multi-tenant deployments in hub-and-spoke architecture. For overall architecture, see [Hub-and-Spoke Overview](hub-spoke-overview.md). For controller implementation, see [Controller to Edge](controller-edge.md).
