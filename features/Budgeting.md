# Midsommar Budget Control System

Below is an **iceberg-style** document describing the Midsommar Budget Control System in both high-level (tip of the iceberg) and in-depth detail. This includes references to the caching mechanism, real-time blocking, monthly usage tracking, automated notifications, and relevant test findings. We also cover the updated **Mermaid diagram** that includes references to notifications. This document aims to help readers—from administrators to AI developers—fully understand how Midsommar's budgeting functionality is implemented and used.

---

## Part 1: The Tip of the Iceberg — High-Level Summary

### Purpose & Objectives

The Midsommar Budget Control System allows organizations to **define and enforce monthly spending caps** on:

1. **Applications (Apps)** that use Large Language Models (LLMs).  
2. **Language Models (LLMs)** themselves (e.g., OpenAI GPT-4).

Key reasons for this functionality:
- **Governance and Audit**: Provides complete visibility into AI usage and ensures compliance with organizational policies.  
- **Cost Control**: Prevents large unexpected bills by imposing monthly budget caps.  
- **Real-Time Blocking**: Immediately denies requests once a budget is exceeded (HTTP 403).  
- **Proactive Alerts**: Triggers notifications (email, in-app) for administrators and/or app owners at key usage thresholds (80% and 100%).  

### Who Uses It (Target Personas)

1. **Administrator (IT/Ops)**:  
   - Sets budgets, monitors usage, receives critical alerts, and ensures overall compliance.  
2. **AI Developer**:  
   - Integrates or builds apps calling LLMs. Relies on budget checks and usage analytics for cost management.  
3. **Chat User (End User)**:  
   - Indirectly interacts (through an App). May see “Over Budget” notifications if usage is exhausted.

### Jobs to Be Done (JTBD)

- **Enforce Governance**: Enforce no app or LLM usage can surpass a set monthly budget, aligning usage with company policy.  
- **Audit & Oversight**: Leverage historical records (`llm_chat_records`) for cost analysis, compliance auditing, or post-incident reviews.  
- **Cost Budgeting**: Maintain monthly budget thresholds for each LLM and app. If usage crosses 100%, further requests are blocked.  
- **Threshold Notifications**: Automatic 80% “warning” and 100% “critical” budget alerts.  
- **Detailed Reporting**: Support a range of analytics endpoints (like `/analytics/budget-usage`) and UI dashboards.

---

## Part 2: Mid-Level Details — Use Cases & Architecture

### Key Features

1. **Monthly Budget Enforcement**  
   - Each app (`apps.monthly_budget`) or LLM (`llms.monthly_budget`) has a monthly limit. If `NULL`, no enforcement is applied.  
   - The budget cycle is determined by `budget_start_date`:
     - If set (e.g., to the 14th), the budget period runs from that day to the same day next month (e.g., Jan 14 to Feb 13).
     - If not set, defaults to the 1st of each month.
   - This allows organizations to align budget cycles with their billing cycles or accounting periods.

2. **Usage Tracking**  
   - Every LLM request is stored in `llm_chat_records`.  
   - The Proxy intercepts each request, calculates cost using model parameters (`model_prices`), then updates `llm_chat_records`.  

3. **Blocking Logic**  
   - If usage already meets or exceeds 100% of budget, requests return:
   ~~~json
   {
     "status": 403,
     "message": "Budget limit exceeded",
     "error": "app monthly budget exceeded: spent 52.34 of 50.00"
   }
   ~~~
   - Similarly for LLM budget.  

4. **Notifications**  
   - At 80% usage: “warning” notifications.  
   - At 100% usage: “critical” notifications.  
   - For apps, the owner (and all admins) receive them.  
   - For LLM budgets, only admins receive them.  

5. **Caching Mechanism**  
   - Budget checks use an in-memory cache that expires (default ~5 minutes).  
   - Minimizes repeated DB queries for each request.  
   - Can be cleared via `BudgetService.ClearCache()` if needed (e.g., if usage data is updated manually).  

### Data Flow & Integration

Below is an updated **Mermaid diagram** showcasing how requests flow, budgets are checked, and notifications are triggered:

~~~mermaid
flowchart LR
    A["Incoming LLM request (App->Proxy)"] --> B["Proxy"]
    B -- "1. CheckBudget(App, LLM)" --> C["BudgetService"]
    C -- "1a. Summarize monthly usage from llm_chat_records (via cache)" --> D[(DB)]
    C -- "2. If usage >= 100% => block & return 403" --> B
    B -- "3. Forward request to LLM vendor" --> E["LLM API"]
    E -- "4. Respond with usage/tokens" --> B
    B -- "5. Insert LLMChatRecord with cost & details" --> D
    C -- "6. AnalyzeBudgetUsage runs periodically or after new records" --> D
    C -- "7. If usage crosses 80% or 100%, trigger notifications" --> G["NotificationService"]
    G -- "8. Send alert to owners/admins" --> H["MailService / UI"]
~~~

**Flow Explanation**:
1. The App calls the Midsommar Proxy with a specific LLM ID/slug.  
2. The Proxy delegates to `BudgetService.CheckBudget(app, llm)`.  
3. If usage is below budget, the request is forwarded to the external LLM.  
4. The LLM vendor returns usage stats (e.g., token counts).  
5. The Proxy calculates cost and writes a record to `llm_chat_records`.  
6. A scheduled or triggered routine (`AnalyzeBudgetUsage`) checks if usage is crossing thresholds.  
7. If 80% or 100% is exceeded, `NotificationService` sends appropriate warnings or critical alerts.  

### System Components

1. **Proxy** (`proxy/`)  
   - Intercepts LLM requests, calls `BudgetService.CheckBudget`, then proxies if allowed.  
   - Records final cost and usage metrics in `llm_chat_records`.  

2. **BudgetService** (`services/budget_service.go`)  
   - **CheckBudget**: Summarizes usage for the month. If usage is ≥ 100%, returns an error for blocking.  
   - **AnalyzeBudgetUsage**: Periodically checks if any app/LLM crosses 80% or 100% threshold and sends notifications.  
   - **Caching**: Maintains a short-lived in-memory usage cache.  

3. **Database**  
   - `apps`: includes `monthly_budget`, `budget_start_date`.  
   - `llms`: same structure for LLM budgets.  
   - `llm_chat_records`: usage log with cost, tokens, timestamps.  
   - `model_prices`: cost per token for each vendor/model.  

4. **NotificationService**  
   - Called by BudgetService at threshold events.  
   - Sends emails or in-app notifications to app owners and/or admins.  

5. **Analytics**  
   - Endpoints like `/analytics/budget-usage` show summarized monthly usage for each app and LLM.

---

## Part 3: Core Features & Governance Functions

### 3.1 Budget Tracking

- **App Budget**  
  - If `apps.monthly_budget` is not `NULL`, the system enforces a limit from `budget_start_date` (or month’s start) through end of month.  
- **LLM Budget**  
  - If `llms.monthly_budget` is not `NULL`, total usage across all apps for that LLM is tracked similarly.  

### 3.2 Governance & Audit

- **Full Usage History**  
  - `llm_chat_records` capture each usage event (cost, tokens, vendor, etc.).  
- **Notifications**  
  - Timely 80% warning and 100% block alerts ensure admins or owners can adapt (e.g., raise budget or investigate usage).  
- **Blocking**  
  - Hard limit at 100% usage.  

### 3.3 Notifications & Alerts

- **80% Threshold**  
  - “Warning” email or UI alert to app owner (for apps) + system admins.  
- **100% Threshold**  
  - “Critical” alert (owner + admins for apps, only admins for LLM budgets).  
  - Budget is considered fully exhausted, blocking further usage.  
- **Integration**  
  - Notification details stored in `notifications` table, read by front-end using `/common/api/v1/notifications` and related endpoints.

### 3.4 Analytics Integration

- **Reports**  
  - `GetBudgetUsage()` aggregator in the BudgetService, returning usage vs. budget for all apps and LLMs.  
- **UI**  
  - The admin dashboard calls `/analytics/budget-usage` or `/analytics/budget-usage-for-app` to display usage bars and color-coded usage statuses (green under 80%, orange near 80%, red at 100%).  

### 3.5 Admin Controls

- **Setting Budgets**  
  - Admin UI or API can set `monthly_budget`.  
  - `budget_start_date` sets a custom monthly cycle:
    - Example: Setting to Jan 14th means budget periods will always run 14th to 13th.
    - The day is extracted and used to calculate current period (e.g., if today is Feb 17th and budget day is 14th, period is Jan 14th to Feb 13th).
    - This ensures consistent budget periods regardless of month lengths.
- **Soft vs. Hard Limit**  
  - Currently the system enforces a hard limit at 100%. If `monthly_budget` is null, no limit.  

### 3.6 Enforcement

- If usage is already over budget, the system returns `403 Forbidden` JSON:
  ~~~json
  {
    "status": 403,
    "message": "Budget limit exceeded",
    "error": "app monthly budget exceeded: spent 50.12 of 50.00"
  }
  ~~~
- This standard message applies to both Apps and LLMs.

---

## Part 4: In-Depth Implementation (Under the Surface)

### 4.1 Database Schema

- **apps**  
  - `monthly_budget FLOAT NULL`  
  - `budget_start_date DATETIME NULL`  
- **llms**  
  - `monthly_budget FLOAT NULL`  
  - `budget_start_date DATETIME NULL`  
- **llm_chat_records**  
  - `id, app_id, llm_id, cost, time_stamp, prompt_tokens, response_tokens, vendor, ...`  
- **model_prices**  
  - `model_name, vendor, cpt, cpit, currency`

### 4.2 Primary Methods

1. **CheckBudget(app, llm)**  
   - Checks usage from the start of the month (or `budget_start_date`) to “now”.  
   - If usage ≥ 100% of budget for either entity, returns an error to block.  
2. **AnalyzeBudgetUsage**  
   - Periodically checks all apps & LLMs for threshold crossing.  
   - Triggers notifications (80% or 100%).  
3. **GetMonthlySpending / GetLLMMonthlySpending**  
   - Aggregates cost from `llm_chat_records`.  
   - Uses in-memory caching (5-minute default).  
4. **NotifyBudgetUsage**  
   - Sends 80% or 100% budget alerts via NotificationService.

### 4.3 Security & Concurrency

- **Proxy Enforcement**  
  - All LLM requests flow via `proxy/proxy.go`, preventing bypass.  
- **Thread-Safe Cache**  
  - A mutex-based approach ensures concurrency safety in the BudgetService.  
- **Concurrent Surges**  
  - If multiple requests simultaneously push usage beyond 100%, some overlap can happen before the system blocks new requests.  

### 4.4 Performance

- **Indexing**  
  - Indexes on `(app_id, time_stamp)` and `(llm_id, time_stamp)` speed up usage queries.  
- **Data Volume**  
  - Large `llm_chat_records` can slow queries; consider partitioning or archiving older records.  
- **Caching**  
  - Reduces repeated DB queries for each request.

---

## Part 5: User Interface & Admin Experience

### 5.1 Admin Dashboard

- **Budget Usage Overview**  
  - Lists each App/LLM with name, monthly budget, spent this month, and usage percentage.  
  - Color-coded usage (green <80%, orange near 80%, red at 100%).  
- **Details**  
  - Clicking on an App shows further analytics (cost/time charts, usage breakdown, associated LLMs).  

### 5.2 App & LLM Forms

- **Create/Update**  
  - Fields to set `monthly_budget` (or leave blank for no limit) and `budget_start_date`.  

### 5.3 Notifications & Emails

- **Budget Alerts**  
  - Email or UI notifications for 80% and 100% usage.  
  - For apps, the owner plus any admins. For LLM budgets, only admins.  

### 5.4 Deletion & Edge Cases

- Deleting an app with an existing monthly budget removes it from the budget checks.  
- If `monthly_budget` is set to zero, it effectively becomes an immediate block unless changed.  
- If usage is updated outside normal flows, `BudgetService.ClearCache()` can be called to re-check from DB.

---

## Part 6: Extended Use Cases & Pitfalls

1. **Stale Cache**  
   - The cache might temporarily hold outdated usage if updated externally. The system auto-refreshes every few minutes.  
2. **Concurrency Overruns**  
   - A surge of requests may slightly exceed budget before the block is enforced.  
3. **Large Data**  
   - If `llm_chat_records` grows large, advanced DB partitioning may be needed for performance.  

---

## Part 7: Future Considerations

1. **Soft Limit**  
   - Option for partial overage (like a grace period) instead of a strict cutoff at 100%.  
2. **Multi-Currency**  
   - Potential for setting budgets in multiple currencies, requiring currency conversion.  
3. **Additional Thresholds**  
   - 50% or 90% notifications, or daily usage updates.  
4. **Enhanced Sub-Org Controls**  
   - Budget partitions by departmental usage or sub-app usage.  
5. **Scalability**  
   - With thousands of usage records per minute, more robust caching or sharding might be necessary.

---

## Conclusion

**Midsommar Budget Control System** tightly enforces monthly budgets on both apps and LLMs, giving:
- **Real-Time Blocking** once 100% usage is hit,  
- **80% & 100% Alerts** for app owners/admins,  
- **Full Audit Trails** via `llm_chat_records`,  
- **In-Flight** usage checks at the Proxy,  
- **Efficient Caching** to minimize DB load,  
- **UI** tools for insights and budget editing.

By combining these features, Midsommar ensures cost predictability, compliance, and governance, while providing the flexibility for advanced use cases and future expansions.
