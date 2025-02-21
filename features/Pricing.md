# Pricing and Cost Management

## Overview
The pricing and cost management system in Midsommar provides a comprehensive solution for tracking, analyzing, and managing costs associated with Large Language Model (LLM) usage. The system supports per-token pricing for input/output operations, cache operations, and includes robust analytics and budgeting features.

## Core Components

### 1. Model Price Structure
Each model price entry contains:
- Model Name (unique per vendor)
- Vendor Name
- Cost per Output Token (CPT)
- Cost per Input Token (CPIT)
- Cache Write Price per Token
- Cache Read Price per Token
- Currency (default: USD)

### 2. Analytics and Usage Tracking
The system tracks detailed usage metrics through `LLMChatRecord`:
- Total tokens (input/output)
- Cache operations (read/write tokens)
- Response time
- Number of choices/tool calls
- Cost per interaction
- User and App association
- Interaction type (chat/proxy)

### 3. Budget Management
Budget tracking is implemented through `BudgetUsage`:
- Entity-level budgets (LLM or App)
- Budget start date
- Current spending
- Usage percentage
- Total cost tracking
- Token consumption monitoring

### 4. Cost Analysis
The system provides extensive cost analysis capabilities:
- Per-day cost analysis by currency
- Cost breakdown by vendor and model
- Token usage statistics
- User-specific cost tracking
- Application-level cost monitoring

## Features

### 1. Price Management
- Create/Update/Delete price configurations
- Bulk price management
- Price retrieval by:
  - ID
  - Model name
  - Vendor
  - Model name and vendor combination

### 2. Default Values
When a new model price is created:
- CPT: 0.0
- CPIT: 0.0
- Cache Write PT: 0.0
- Cache Read PT: 0.0
- Currency: "USD"

### 3. Analytics Features
#### Usage Statistics
- Chat records per day
- Tool calls per day
- Unique users per day
- Token usage per user/app
- Model usage statistics
- Vendor usage tracking

#### Cost Analysis
- Daily cost breakdown
- Currency-specific analysis
- Interaction type filtering
- Historical cost trends
- Budget vs actual spending

#### Chat/Proxy Logs
- Detailed interaction logs
- Response time tracking
- Token usage breakdown
- Cost per interaction
- Cache operation metrics

### 4. API Endpoints

#### Price Management
```
POST    /model-prices              # Create price
GET     /model-prices/{id}         # Get price by ID
PATCH   /model-prices/{id}         # Update price
DELETE  /model-prices/{id}         # Delete price
GET     /model-prices              # List all prices
GET     /model-prices/by-vendor    # Get prices by vendor
GET     /model-prices/by-name      # Get/create price by name
```

#### Analytics
```
GET     /analytics/cost            # Cost analysis
GET     /analytics/usage           # Usage statistics
GET     /analytics/budget          # Budget tracking
GET     /analytics/logs            # Interaction logs
```

## User Interface

### 1. Model Prices View
- Table display of all model prices
- Add/Edit/Delete operations
- Pagination support
- Vendor filtering
- Price conversion display (per token/per million tokens)

### 2. Analytics Dashboard
- Cost trends visualization
- Usage statistics charts
- Budget tracking displays
- Token consumption metrics
- Interactive data filtering

### 3. Price Configuration Form
- Model name input
- Vendor selection
- Cost input fields
- Currency selection
- Validation rules

## Integration Points

### 1. Core System Integration
- AI Gateway cost tracking
- Chat system integration
- Proxy system cost logging
- Cache system cost tracking

### 2. Analytics Integration
- Real-time cost tracking
- Usage statistics collection
- Budget monitoring
- Performance metrics

### 3. External Systems
- Billing system integration
- Reporting system feeds
- Monitoring system alerts
- Audit logging

## Security
- Authentication required for all endpoints
- Role-based access control
- Admin-only price management
- Secure API token handling

## Related Files

### Core Models and Logic
- `models/prices.go` - Core price model definitions and methods
- `models/prices_test.go` - Tests for price models
- `models/analytics.go` - Analytics data models
- `models/budget.go` - Budget tracking models

### Services
- `services/prices.go` - Price management service
- `services/prices_service.go` - Price service implementation
- `services/prices_service_test.go` - Price service tests
- `services/budget_service.go` - Budget management service
- `services/budget_service_test.go` - Budget service tests

### API Handlers
- `api/prices_handlers.go` - Price management API endpoints
- `api/prices_handlers_test.go` - Tests for price endpoints
- `api/analytics_handlers.go` - Analytics API endpoints
- `api/analytics_handlers_test.go` - Tests for analytics endpoints

### Analytics
- `analytics/analytics.go` - Core analytics functionality
- `analytics/analytics_test.go` - Analytics tests
- `proxy/proxy_analytics_test.go` - Proxy analytics tests
- `proxy/proxy_budget_test.go` - Proxy budget tests

### Frontend Components
- `ui/admin-frontend/src/admin/pages/ModelPriceList.js` - Price list page
- `ui/admin-frontend/src/admin/components/model-prices/ModelPriceForm.js` - Price form component
- `ui/admin-frontend/src/admin/utils/budgetFormatter.js` - Budget formatting utilities

### Documentation
- `docs/site/content/docs/model-prices.md` - User documentation
- `features/Budgeting.md` - Budgeting feature specification

### Templates
- `templates/budget_alert.tmpl` - Budget alert email template

## Database Schema
```sql
-- Model Prices
CREATE TABLE model_prices (
    id SERIAL PRIMARY KEY,
    model_name VARCHAR NOT NULL,
    vendor VARCHAR NOT NULL,
    cpt FLOAT NOT NULL,
    cpit FLOAT NOT NULL,
    cache_write_pt FLOAT NOT NULL,
    cache_read_pt FLOAT NOT NULL,
    currency VARCHAR NOT NULL,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    UNIQUE(model_name, vendor)
);

-- Usage Records
CREATE TABLE llm_chat_records (
    id SERIAL PRIMARY KEY,
    name VARCHAR NOT NULL,
    vendor VARCHAR NOT NULL,
    llm_id INTEGER,
    total_time_ms INTEGER,
    prompt_tokens INTEGER,
    response_tokens INTEGER,
    total_tokens INTEGER,
    time_stamp TIMESTAMP,
    user_id INTEGER,
    choices INTEGER,
    tool_calls INTEGER,
    chat_id VARCHAR,
    app_id INTEGER,
    cost FLOAT,
    currency VARCHAR,
    interaction_type VARCHAR,
    cache_write_prompt_tokens INTEGER,
    cache_read_prompt_tokens INTEGER,
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
