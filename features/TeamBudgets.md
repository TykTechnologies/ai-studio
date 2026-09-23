# Team Budgets (Enterprise)

Team budgets add two things on top of the per-App and per-LLM budgets described in [Budgeting.md](Budgeting.md):

1. **Team cost reporting**: spend per team (`models.Group`, shown as "Teams" in the UI) over any date range, for every team, whether or not it has a budget.
2. **Team budget setting**: a monthly budget per team that works as both
   - a **ceiling** on the combined spend of the team's Apps, and
   - an **allocation pool** that new Apps draw their App budget from.

Team budgets are an Enterprise feature. Community Edition still attributes Apps and spend to teams (so history is in place after an upgrade), but it allocates and enforces nothing, and the team budget endpoints answer 402.

## Budget values: no limit versus zero (breaking change)

This applies to every budget: App, LLM and team.

| Value | Meaning |
|---|---|
| `null` (empty) | **No limit.** |
| `0` | **A budget of zero.** Nothing may be spent, and requests are refused. |
| `> 0` | The monthly limit. |

Before this change, any budget at or below 0 meant "no limit".

**Upgrade:**
- A one-time migration (`models.ClearLegacyZeroBudgets`, recorded in `team_budget_settings.zero_budgets_cleared`) rewrites stored App and LLM budgets of 0 to `null`, so nothing starts blocking.
- API clients that send `monthly_budget: 0` to mean "no limit" must send `null`, or omit the field, instead.

**Creating Apps:**
- A new App with no budget (`null`) gets whatever default applies: the team's allocation when its team has a pool, else `DEFAULT_APP_BUDGET`, else no limit.
- An explicit 0 is kept as 0.

**UI:** budget fields are an explicit choice. On a new App it is "Default" or a fixed amount; everywhere else it is "No limit" or a fixed amount. Displays show "No limit" and "$0.00 (requests are refused)" as different things.

**Edges (microgateway):**
- The synced App config, and the microgateway's own App table, still read a budget at or below 0 as "no limit". Changing that inside the gateway would break its standalone management API, its CLI and its built-in admin App.
- A zero budget therefore reaches edges as a block (see [Edge gateways](#edge-gateways-microgateway)).
- Standalone microgateways (no Studio) keep their existing convention.

## Attribution: which team an App belongs to

Users can belong to many teams, are always in Default, and SSO rewrites their memberships at every login. So the team is **stamped on the App when it is created** (`apps.team_id`) and is never recomputed from live membership.

The team is resolved from the owner (`models.ResolveBudgetTeam`):

1. the owner's **budget team** (`users.budget_team_id`), while they are still a member of it (a stale choice left behind by an SSO rewrite is skipped);
2. otherwise the owner's first team other than Default, by lowest team ID;
3. otherwise the **Default** team.

Administrators can pick another team when creating or editing an App (`team_id` on the App API). A user's budget team is set on the user form, or with `PUT /api/v1/users/:id/budget-team`, and must be one of their teams. Deleting a team clears it as anyone's budget team; its Apps and past spend keep the attribution for reporting.

**Spend records** (`llm_chat_records.team_id`) are stamped by the analytics writer before they are stored. That writer is the path for embedded-gateway traffic, edge traffic arriving in the analytics pulse, and chat.

- Proxy and edge traffic take their App's team.
- Chat, which has no App, takes the chatting user's resolved team.

Because the team is stamped at write time, moving an App to another team does not rewrite its history.

The first boot after the upgrade backfills `apps.team_id` and `llm_chat_records.team_id` once (`models.BackfillTeamAttribution`).

## Settings

| Setting | Where | Meaning |
|---|---|---|
| Global switch | `team_budget_settings.enabled`; Teams page; `GET/PUT /api/v1/team-budgets/settings` | Off by default. While off, nothing is allocated or enforced; attribution and reporting still work. Switching it on gives the Default team a budget of 0 unless it already has one. |
| Monthly budget | `team_budgets.monthly_budget` | `null`: the team is **unmanaged** (no pool, no ceiling). `0`: an empty pool and a zero ceiling. `> 0`: the pool and the ceiling. |
| Default App allocation | `team_budgets.default_app_allocation` | What a new App draws from the pool when it is created with no budget. It is capped at what is left unallocated, so it can be 0. |
| Enforcement | `team_budgets.enforcement` | `alert_only` (default): notify only. `hard_block`: refuse every App of the team once its spend reaches the budget. For a budget of 0 that is immediately. |
| Budget start date | `team_budgets.budget_start_date` | Periods run monthly from this day of the month (the 1st when empty). Anchor days past the end of a short month fall on its last day. A reset starts the period at the reset itself. |

With the switch on, the Default team's budget of 0 means:
- Apps created for users in no other team get 0 and are refused until an administrator allocates to them.
- An alert-only Default team also raises its 100% alert as soon as Apps it holds that have no limit spend anything.

## Allocation

Allocation applies only while the switch is on and the App's team is managed.

- **New App, no budget requested:** the App gets `min(default allocation, unallocated pool)`. The team default takes precedence over `DEFAULT_APP_BUDGET`.
- **New App with a budget** (admin form, portal, SDK): the budget must fit the unallocated pool, or creation fails with 400.
- **Editing a budget, or moving an App to another team:** the new amount must fit the destination pool. Keeping or lowering an allocation always works, and so does setting "No limit". An App with no limit holds no share of the pool, but its spend still counts towards the team ceiling.
- **Allocated** is the sum of the budgets of the team's live Apps with a budget above 0. Budgets set before the switch count too.

### Decommissioning

- **Deleting an App** (soft delete) drops it out of the allocated sum, so its allocation returns to the pool at once. The spend it made this period stays in the team's spend, so deleting and recreating Apps cannot beat the ceiling.
- **Deleting a user** orphans their Apps. An orphaned App in a managed team gets a budget of 0, which releases its allocation; its credential is already deactivated.

## Enforcement

### Embedded gateway (inside Studio)

`budget.Service.CheckBudget` (Enterprise) consults the team through the `budget.TeamChecker` hook (`Service.InitBudgets` wires it), so every call site of the embedded gateway enforces team budgets. A request is refused with 403 when:

- the App's or the LLM's own budget is 0, or has been reached; or
- the App's team is `hard_block` and its spend this period (all of its Apps including deleted ones, plus its members' chat) has reached the team budget.

The API and the embedded gateway share one budget service, so resets clear the cache the embedded gateway reads. They used to be separate instances.

### Edge gateways (microgateway)

Edge traffic never passes Studio's budget check. Edges enforce App budgets locally from the synced App config and their own `budget_usage`. On top of that, the budget sync (`budget.sync`, every 30 s, `BUDGET_SYNC_INTERVAL`) now carries `blocks`: the complete map of App ID to reason for Apps that must be refused (`EdgeBlocks` on the Enterprise budget service). An App is listed when:

- its budget is 0, or
- its team is `hard_block` and has reached its budget.

**How edges handle the list:**
- It is sent even when empty, so edges release Apps that are allowed again, for example after a team reset or a budget raise.
- Edges replace their set on each sync, store it in their `budget_blocks` table (so a restarted edge keeps refusing), and refuse listed Apps in the Enterprise `CheckBudget` before any local check.
- Community Edition controls and older controls don't send `blocks`, so an edge keeps whatever set it has.

**Version compatibility:**
- The field is ignored by edges that predate it. A zero-budget App on an old edge is not refused until that edge is upgraded; everything else behaves as before.
- There is no protobuf change and no config checksum churn.

**Lag:** a new block reaches edges within one pulse interval plus one sync interval. Edge spend has to reach Studio in the analytics pulse, and then the next budget sync carries the block. With the defaults that's up to about 40 s.

Edge App-budget enforcement itself is unchanged. In live testing an App overshot its own budget by a few requests before the edge refused it; that comes from the edge's existing usage accounting, not from this feature.

### Alerts for edge spend

Studio's alerts used to run only after embedded-gateway requests, so microgateway traffic never raised App, LLM or team budget alerts. Now, on every budget sync, Studio passes the Apps whose edge spend moved since the previous sync to `AnalyzeApps`. That re-reads their spend and raises any alerts due for the App, its LLMs and its team.

### Other notes

- **Caching (Studio):** the switch and team rows are cached for 30 s per node, and team spend for 30 s. Allocation is serialised per node only.
- **Chat** is still not budget-checked (unchanged); chat spend counts towards the team's spend and reports.
- **Currency:** team budgets assume one currency, as App budgets do.

## Knowing when a team overshoots

`GET /api/v1/groups/:id/budget` returns three signals, and the team page shows them:

- **`over_budget`:** spend this period ≥ the budget (any spend against a budget of 0). At 80% and again at 100%, once per period per budget amount:
  - an email and in-app notification goes to administrators (`templates/team_budget_alert.tmpl`);
  - a `budget.team.threshold` event is published on the local bus and is available to webhooks;
  - a SYSTEM audit record is written.
  - With a budget of 0 there is only the 100% alert.
- **`blocking`:** the team hard-blocks and its Apps are being refused right now.
- **`over_allocated`:** App allocations add up to more than the budget, for example after it was lowered. This is a warning, not a block. `budget.team.over_allocated` fires when a budget change causes it.

## API

| Method and path | Permission | Purpose |
|---|---|---|
| `GET /api/v1/team-budgets/settings` | groups read | Global switch |
| `PUT /api/v1/team-budgets/settings` | groups write | `{"enabled": bool}` |
| `GET /api/v1/groups/:id/budget` | groups read | Report: budget, period, spent, chat spent, allocated/unallocated, flags, per-App rows (including deleted Apps that spent this period) |
| `PUT /api/v1/groups/:id/budget` | groups write | `{monthly_budget, default_app_allocation, enforcement, budget_start_date}` |
| `DELETE /api/v1/groups/:id/budget` | groups delete | Make the team unmanaged |
| `POST /api/v1/groups/:id/budget/reset` | groups write | Start a new period now |
| `GET /api/v1/analytics/team-costs?start_date&end_date` | analytics read | Cost, tokens and requests per team (every live team, plus deleted teams that spent), plus unattributed spend |
| `PUT /api/v1/users/:id/budget-team` | users write | `{"team_id": id or null}` |

- App create/update accept `team_id`, and App responses carry it.
- The portal App detail response carries `budget_source: "team"` when the App's team hands out budgets from a pool, so the portal can say where the budget came from.
- User responses carry `budget_team_id`.

## UI

- **Teams page:** the global switch.
- **Team detail:** a Budget panel with the stats, spent and allocated bars, blocking/over-budget/over-allocated alerts and the per-App table (allocation, spend, and "No limit", "Budget $0: refused" or "Decommissioned"), plus an editor, reset and remove.
- **App form:** the budget mode field, a team picker with the pool summary, and the server's reason when an allocation is refused.
- **App detail:** the team, and a note when the team has a pool.
- **User form:** a budget team picker (shown when the user is in more than one team).
- **LLM form:** the budget mode field.
- **Dashboard:** a Team Costs table for the selected date range.

## Code map

- **Models:** `models/team_budget.go` (`TeamBudget`, `TeamBudgetSettings`, `ResolveBudgetTeam`, `BackfillTeamAttribution`, `ClearLegacyZeroBudgets`), plus fields in `models/app.go`, `models/user.go` and `models/analytics.go`.
- **Contract and CE stub:** `services/team_budget/`. Wiring: `Service.InitBudgets` (`services/service.go`). App lifecycle: `services/app_team_budget.go`, `services/app.go`, `services/user_service.go`, `services/group_service.go`.
- **Enterprise:** `enterprise/features/team_budget/`, and `enterprise/features/budget/service.go` (zero-budget enforcement, `EdgeBlocks`, `AnalyzeApps`, team hook).
- **Spend stamping:** `analytics/team_stamp.go`.
- **Edge sync:** `grpc/budget_sync_service.go` (`EdgeBudgetSource`, blocks, moved-spend analysis), `microgateway/internal/services/budget_blocks.go`, `budget_sync_handler.go`, `budget_service.go`, and `microgateway/internal/database/models.go` (`BudgetBlock`).
- **API:** `api/team_budget_handlers.go`.

## Follow-ups

- SSO profiles cannot yet set a user's budget team from a claim.
- Teams have no team-admin role; only platform roles with groups:write manage team budgets.
- Chat is not enforced against budgets (App, LLM or team).
- LLM budgets are not enforced on edges (unchanged; they are enforced in the embedded gateway).
- Edge App-budget enforcement can overshoot by a few requests under steady traffic (pre-existing; seen in live testing).
