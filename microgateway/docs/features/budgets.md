# Budget Management

The microgateway provides comprehensive budget management capabilities to control AI/LLM costs and prevent budget overruns.

## Overview

Budgets are enforced per **application**:
- **Application budgets**: a monthly spending limit per application, covering all its LLM usage
- **Pre-request enforcement**: requests are refused once the application has spent its budget (Enterprise Edition)
- **Flexible reset cycles**: configurable monthly reset dates

Budget *enforcement* is an Enterprise Edition feature. Community Edition records usage for reporting but never refuses a request.

When the microgateway runs as an edge of AI Studio (hub-and-spoke), budgets are managed in AI Studio; see [Budgets on edge gateways](#budgets-on-edge-gateways-ai-studio).

## Budget Types

### Application Budgets
Each application can have a monthly budget limit that applies to all LLM usage for that application.

```bash
# Set application budget
mgw app create \
  --name="My App" \
  --email=user@company.com \
  --budget=500.0 \
  --reset-day=1

# Update existing budget
mgw app update 1 --budget=1000.0 --reset-day=15
```

### LLM Budgets
LLM configurations carry a `monthly_budget` field (`mgw llm create --budget=...`), and it is stored and returned by the API. It is **not enforced** by the microgateway: only application budgets are. To cap spend on an expensive provider, give the applications that use it their own budgets, or enforce the LLM budget in AI Studio's embedded gateway.

## Budget Enforcement

### Pre-Request Validation
Before each LLM request, the microgateway (Enterprise Edition):
1. Refuses the request if AI Studio has told this gateway to block the application (see below)
2. Looks up the application's spend so far in the current budget period
3. Refuses the request if that spend has reached the application's budget
4. Answers `403 Forbidden` ("Budget limit exceeded") when it refuses

The check uses spend already recorded; the cost of the incoming request is not estimated. Spend is recorded after each response, so under steady traffic an application can go a few requests past its budget before requests are refused.

### Cost Calculation
The cost of each request is calculated after the response, from its prompt and completion token counts (including cache tokens) and the model's configured prices.

## Budget Configuration

### Monthly Reset Cycles
```bash
# Reset on 1st of each month (default)
mgw app create --name="App" --budget=1000.0 --reset-day=1

# Reset on 15th of each month
mgw app create --name="App" --budget=1000.0 --reset-day=15

# Valid reset days: 1-28
```

### Budget Limits
```bash
# Set specific budget amount
--budget=1000.0

# Unlimited budget (no enforcement)
--budget=0

# Or omit budget parameter for unlimited
```

On a standalone microgateway a budget of 0 means "no limit". AI Studio works differently: there an empty budget means no limit and 0 means nothing may be spent. The edge applies AI Studio's zero budgets through the blocks described below.

## Monitoring Budget Usage

### Current Usage
```bash
# Check application budget status
mgw budget usage 1

# Example output:
# APP_ID  USAGE    BUDGET   REMAINING  % USED
# 1       $45.75   $500.00  $454.25    9.2%
```

### Budget History
```bash
# Get budget history
mgw budget history 1

# History for specific time range
mgw budget history 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z
```

### Usage Analytics
```bash
# View cost breakdown
mgw analytics costs 1

# Detailed cost analysis
mgw analytics costs 1 \
  --start=2024-01-01T00:00:00Z \
  --end=2024-01-31T23:59:59Z
```

## API Integration

### Budget Status API
```bash
# Get budget usage via API
curl -H "Authorization: Bearer $ADMIN_TOKEN" \
  "http://localhost:8080/api/v1/budgets/1/usage"

# Response:
{
  "data": {
    "app_id": 1,
    "monthly_budget": 500.0,
    "current_usage": 45.75,
    "remaining_budget": 454.25,
    "percentage_used": 9.15,
    "is_over_budget": false,
    "period_start": "2024-01-01T00:00:00Z",
    "period_end": "2024-01-31T23:59:59Z"
  }
}
```

### Budget Update API
```bash
# Update budget via API
curl -X PUT \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"monthly_budget": 1000.0, "budget_reset_day": 15}' \
  "http://localhost:8080/api/v1/budgets/1"
```

## Budgets on Edge Gateways (AI Studio)

When the microgateway runs in edge mode, AI Studio is the source of truth for budgets:

- **Application budgets and periods** arrive with the application configuration pushed from AI Studio.
- **Spend** is recorded locally and sent to AI Studio in the analytics pulse. AI Studio adds up spend from every edge and sends each application's total back in the budget sync (`budget.sync`, every 30 seconds by default, `BUDGET_SYNC_INTERVAL` on AI Studio). Each edge keeps the higher of its own figure and AI Studio's, so one edge cannot spend what another already has.
- **Blocks:** the same sync lists applications every edge must refuse whatever its local numbers say:
  - applications whose budget in AI Studio is 0;
  - applications whose team has spent a blocking team budget (AI Studio team budgets).

  The list replaces the previous one on every sync, so an application is served again once it is allowed again, for example after a team budget reset. Edges store the list in their `budget_blocks` table, so a restarted edge keeps refusing until the next sync.
- **Alerts:** AI Studio raises budget alerts (80% and 100%) for spend that arrives from edges, as it does for its own traffic.

Blocks reach edges with a delay of up to one pulse interval plus one sync interval after the spend that causes them.

## Budget Scenarios

### Development Teams
```bash
# Small development budget
mgw app create \
  --name="Dev Team A" \
  --email=dev-team-a@company.com \
  --budget=100.0 \
  --reset-day=1
```

### Production Applications
```bash
# Larger production budget
mgw app create \
  --name="Production App" \
  --email=ops@company.com \
  --budget=5000.0 \
  --reset-day=1
```

### Cost-Conscious Testing
```bash
# Low budget for testing
mgw app create \
  --name="Testing Environment" \
  --email=qa@company.com \
  --budget=50.0 \
  --reset-day=1
```

## Budget Alerts and Monitoring

### Usage Tracking
The microgateway tracks:
- Total tokens consumed
- Cost per request
- Cumulative monthly spending
- Budget utilization percentage

### Budget Thresholds
Monitor budget usage with CLI:
```bash
# Daily budget check script
#!/bin/bash
USAGE=$(mgw budget usage 1 --format=json | jq '.data.percentage_used')
if (( $(echo "$USAGE > 80" | bc -l) )); then
  echo "Warning: Budget usage at ${USAGE}%"
fi
```

### Cost Optimization
```bash
# Analyze cost patterns (per LLM, for reporting)
mgw analytics costs 1 --format=json | \
  jq '.data.cost_by_llm'

# Find high-cost requests
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.cost > 1.0)'
```

## Configuration

### Settings
Budget enforcement has no switches of its own: it applies in Enterprise Edition whenever an application has a budget above 0. Costs come from the model prices configured on the gateway (or synced from AI Studio).

### Database Schema
Budget data is stored in these tables:
- `budget_usage` - Spend per application and budget period
- `budget_blocks` - Applications AI Studio says to refuse (edge mode)
- `analytics_events` - Individual request costs
- `apps` - Application budget settings

## Best Practices

### Budget Planning
- Start with conservative budgets and adjust based on usage
- Set different budgets for development vs. production
- Use separate applications (each with its own budget) to allocate cost per team, project or provider
- Monitor usage patterns to optimize budgets

### Cost Control
- Use cheaper models for development and testing
- Implement request batching where possible
- Monitor token usage patterns
- Set up alerts for budget thresholds

### Team Management
- Separate budgets per team or project
- Use different applications for different environments
- Implement approval workflows for budget increases
- Regular budget reviews and adjustments

## Troubleshooting

### Budget Not Enforcing
```bash
# Check budget configuration and spend
mgw budget usage 1

# Check cost calculation
mgw analytics events 1 | grep cost
```

- Enforcement needs Enterprise Edition; Community Edition only records usage.
- A budget of 0 on a standalone gateway means "no limit".
- LLM budgets are not enforced on the gateway; set application budgets.
- In edge mode, a block from AI Studio takes up to one pulse plus one sync interval to arrive.

### Inaccurate Cost Tracking
```bash
# Check model pricing configuration
mgw analytics costs 1 --format=json

# Review recent events for cost data
mgw analytics events 1 --limit=10
```

### Budget Reset Issues
```bash
# Check reset day configuration
mgw app get 1 | grep reset_day

# Periods start at midnight (gateway local time) on the reset day
```

---

Budget management provides essential cost control for AI/LLM usage. For detailed analytics, see [Analytics](analytics.md). For application management, see [Apps](apps.md).
