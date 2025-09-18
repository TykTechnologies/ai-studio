# Budget Management

The microgateway provides comprehensive budget management capabilities to control AI/LLM costs and prevent budget overruns.

## Overview

Budget management operates at multiple levels:
- **Application-level budgets**: Overall spending limits per application
- **LLM-specific budgets**: Spending limits per LLM provider
- **Real-time enforcement**: Pre-request budget validation
- **Flexible reset cycles**: Configurable monthly reset dates

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

### LLM-Specific Budgets
Individual LLM providers can have their own budget limits.

```bash
# Create LLM with budget
mgw llm create \
  --name="GPT-4" \
  --vendor=openai \
  --model=gpt-4 \
  --api-key=$OPENAI_API_KEY \
  --budget=300.0

# Update LLM budget
mgw llm update 1 --budget=500.0
```

## Budget Enforcement

### Pre-Request Validation
Before each LLM request, the microgateway:
1. Estimates the cost of the request
2. Checks against remaining budget
3. Blocks the request if it would exceed the budget
4. Returns `402 Payment Required` if over budget

### Cost Estimation
The microgateway estimates costs based on:
- Input token count
- Expected output token ratio
- LLM provider pricing models
- Historical usage patterns

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

## Monitoring Budget Usage

### Current Usage
```bash
# Check application budget status
mgw budget usage 1

# Check budget for specific LLM
mgw budget usage 1 --llm-id=2

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

# History for specific LLM
mgw budget history 1 --llm-id=2
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
# Analyze cost patterns
mgw analytics costs 1 --format=json | \
  jq '.data.cost_by_llm'

# Find high-cost requests
mgw analytics events 1 --format=json | \
  jq '.data[] | select(.cost > 1.0)'
```

## Configuration

### Environment Variables
Budget-related configuration options:

```bash
# Budget enforcement settings
BUDGET_CHECK_ENABLED=true
BUDGET_ESTIMATION_BUFFER=0.1  # 10% safety margin
BUDGET_RESET_TIMEZONE=UTC

# Cost tracking
COST_CALCULATION_ENABLED=true
COST_PRECISION=4  # Decimal places for cost calculations
```

### Database Schema
Budget data is stored in these tables:
- `budget_usage` - Current usage tracking
- `analytics_events` - Individual request costs
- `apps` - Application budget settings
- `llms` - LLM budget settings

## Best Practices

### Budget Planning
- Start with conservative budgets and adjust based on usage
- Set different budgets for development vs. production
- Use LLM-specific budgets for cost allocation
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
# Check budget configuration
mgw budget usage 1

# Verify budget enforcement is enabled
# Check BUDGET_CHECK_ENABLED=true in configuration

# Check cost calculation
mgw analytics events 1 | grep cost
```

### Inaccurate Cost Tracking
```bash
# Verify cost calculation is enabled
# Check COST_CALCULATION_ENABLED=true

# Check model pricing configuration
mgw analytics costs 1 --format=json

# Review recent events for cost data
mgw analytics events 1 --limit=10
```

### Budget Reset Issues
```bash
# Check reset day configuration
mgw app get 1 | grep reset_day

# Verify timezone settings
# Check BUDGET_RESET_TIMEZONE in configuration
```

---

Budget management provides essential cost control for AI/LLM usage. For detailed analytics, see [Analytics](analytics.md). For application management, see [Apps](apps.md).
