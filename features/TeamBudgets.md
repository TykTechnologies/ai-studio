# Team Budgets (Enterprise)

Team budgets add two things on top of the per-App and per-LLM budgets described in [Budgeting.md](Budgeting.md):

1. **Team cost reporting**: spend per team (`models.Group`, shown as "Teams" in the UI) over any date range, for every team, whether or not it has a budget.
2. **Team budget setting**: a monthly budget per team that works as both
   - a **ceiling** on the combined spend of the team's Apps, and
   - an **allocation pool** that new Apps draw their App budget from.

Team budgets are an Enterprise feature. Community Edition still attributes Apps and spend to teams (so history is in place after an upgrade), but it allocates and enforces nothing, and the team budget endpoints answer 402.

## Attribution: which team an App belongs to

Users can belong to many teams, are always in Default, and SSO rewrites their memberships at every login. So the team is **stamped on the App when it is created** (`apps.team_id`) and is never recomputed from live membership.

The team is resolved from the owner (`models.ResolveBudgetTeam`):

1. the owner's **budget team** (`users.budget_team_id`), while they are still a member of it (a stale choice left behind by an SSO rewrite is skipped);
2. otherwise the owner's first team other than Default, by lowest team ID;
3. otherwise the **Default** team.

Administrators can pick another team when creating or editing an App (`team_id` on the App API). A user's budget team is set on the user form, or with `PUT /api/v1/users/:id/budget-team`, and must be one of their teams. Deleting a team clears it as anyone's budget team; its Apps and past spend keep the attribution for reporting.

**Spend records** (`llm_chat_records.team_id`) are stamped by the analytics writer before they are stored:

- proxy and edge traffic take their App's team;
- chat, which has no App, takes the chatting user's resolved team.

Because the team is stamped at write time, moving an App to another team does not rewrite its history.

The first boot after the upgrade backfills `apps.team_id` and `llm_chat_records.team_id` once (`models.BackfillTeamAttribution`; `team_budget_settings.attribution_backfilled` records that it ran). Existing App budgets are untouched.

## Settings

| Setting | Where | Meaning |
|---|---|---|
| Global switch | `team_budget_settings.enabled`; Teams page; `GET/PUT /api/v1/team-budgets/settings` | Off by default. While off, nothing is allocated or enforced; attribution and reporting still work. Switching it on gives the Default team an empty pool (budget 0) unless it already has a budget. |
| Monthly budget | `team_budgets.monthly_budget` | nil: the team is **unmanaged** (no pool, no ceiling). 0: **managed, empty pool**; new Apps are allocated nothing. Above 0: the pool and the ceiling. |
| Default App allocation | `team_budgets.default_app_allocation` | What a new App draws from the pool when no budget is requested. It is capped at what is left unallocated. |
| Enforcement | `team_budgets.enforcement` | `alert_only` (default): notify only. `hard_block`: refuse every App of the team once the ceiling is reached. |
| Budget start date | `team_budgets.budget_start_date` | Periods run monthly from this day of the month (the 1st when empty). Anchor days past the end of a short month fall on its last day. A reset starts the period at the reset itself. |

## Allocation

Allocation applies only while the switch is on and the App's team is managed.

- **New App, no budget requested:** the App gets `min(default allocation, unallocated pool)` and `apps.budget_source = "team"`. The team default takes precedence over `DEFAULT_APP_BUDGET`.
- **New App with a requested budget** (admin form, portal, SDK): the request must fit the unallocated pool, or creation fails with 400. The App is then team-allocated.
- **Editing a budget, or moving an App to another team:** the new amount must fit the destination pool. Keeping or lowering an allocation always works.
- **Allocated** is the sum of the budgets of the team's live Apps with a budget above 0, whatever their source. Manually budgeted Apps take their share of the pool too.
- **`budget_source` explains what a zero budget means:**
  - `""` (manual, and every App created before the switch): the existing rule holds, so ≤ 0 means unlimited.
  - `"team"`: ≤ 0 means nothing is allocated, and the App is **refused** ("team allocation exhausted").
- **Adopting:** `POST /api/v1/apps/:id/adopt-team-budget` ("Move into team pool" on the App page) turns a manually budgeted App into a team allocation. It keeps its current budget, or takes the team default if it has none, provided that fits the pool.

### Decommissioning

- **Deleting an App** (soft delete) drops it out of the allocated sum, so its allocation returns to the pool at once. The spend it made this period stays in the team's spend, so deleting and recreating Apps cannot beat the ceiling.
- **Deleting a user** orphans their Apps. The allocation of an orphaned App in a managed team is released (budget 0, source `team`); its credential is already deactivated.

## Enforcement

`budget.Service.CheckBudget` (Enterprise) consults the team first through the `budget.TeamChecker` hook (`Service.InitBudgets` wires it). Every proxy call site therefore enforces team budgets, including failover rungs and the Bedrock translators. A request is refused with 403 when:

- the App is team-allocated with nothing allocated, or
- the team is `hard_block`, has a budget above 0, and its spend this period (all of its Apps including deleted ones, plus its members' chat) has reached the budget.

**Caching:**
- The switch and team rows are cached for 30 s on each Studio node; writes on the same node clear the cache at once.
- Team spend is cached for 30 s and refreshed synchronously on a miss (one indexed query on `(team_id, time_stamp)`).
- A team can therefore overshoot by up to 30 s of traffic.
- Allocation is serialised per node only, so two Studio nodes creating Apps in the same team at the same instant could both take the last of a pool.

**Edges (microgateway):**
- The 30 s budget sync (`budget.sync`) carries `team_blocks`: the complete map of App ID → reason from `team_budget.Service.EdgeBlocks`. It is sent even when empty, so edges release Apps whose team is back under budget.
- Edges keep the set in memory and refuse listed Apps in the Enterprise `CheckBudget`. A restarted edge enforces no team blocks until the first sync.
- Edge enforcement uses the control plane's view of spend, so edge overshoot is bounded by the sync interval.
- No protobuf change was needed.

**Chat** is still not budget-checked (unchanged). Chat spend counts towards the team's spend and reports.

**Currency:** team budgets assume one currency, as App budgets do.

## Knowing when a team overshoots

There are two distinct signals, both shown on the team page and returned by `GET /api/v1/groups/:id/budget`:

- **Over budget:** spend this period ≥ budget. At 80% and again at 100%, once per period per budget amount:
  - an email and in-app notification goes to administrators (`templates/team_budget_alert.tmpl`);
  - a `budget.team.threshold` event is published on the local bus and is available to webhooks;
  - a SYSTEM audit record is written ("Team budget reached 80%" / "Team budget exceeded, team Apps blocked").
- **Over-allocated:** App allocations add up to more than the budget, e.g. after an admin lowered the budget. This is a warning, not a block. A `budget.team.over_allocated` event fires when a budget change causes it.

## API

| Method and path | Permission | Purpose |
|---|---|---|
| `GET /api/v1/team-budgets/settings` | groups read | Global switch |
| `PUT /api/v1/team-budgets/settings` | groups write | `{"enabled": bool}` |
| `GET /api/v1/groups/:id/budget` | groups read | Report: budget, period, spent, chat spent, allocated/unallocated, flags, per-App rows (including deleted Apps that spent this period) |
| `PUT /api/v1/groups/:id/budget` | groups write | `{monthly_budget, default_app_allocation, enforcement, budget_start_date}` |
| `DELETE /api/v1/groups/:id/budget` | groups delete | Make the team unmanaged |
| `POST /api/v1/groups/:id/budget/reset` | groups write | Start a new period now |
| `POST /api/v1/apps/:id/adopt-team-budget` | apps write | Move a manual App budget into the pool |
| `GET /api/v1/analytics/team-costs?start_date&end_date` | analytics read | Cost, tokens and requests per team (every live team, plus deleted teams that spent), plus unattributed spend |
| `PUT /api/v1/users/:id/budget-team` | users write | `{"team_id": id or null}` |

App create/update accept `team_id`, and App responses (admin and portal) carry `team_id` and `budget_source`. User responses carry `budget_team_id`.

## UI

- **Teams page:** the global switch.
- **Team detail:** a Budget panel with the stats, spent and allocated bars, overshoot alerts and the per-App table, plus an editor, reset and remove.
- **App form:** a team picker with the pool summary; a refused allocation shows the server's reason.
- **App detail:** the team, the allocation badge and "Move into team pool".
- **User form:** a budget team picker (shown when the user is in more than one team).
- **Dashboard:** a Team Costs table for the selected date range.

## Code map

- Models: `models/team_budget.go` (`TeamBudget`, `TeamBudgetSettings`, `ResolveBudgetTeam`, `BackfillTeamAttribution`), plus fields in `models/app.go`, `models/user.go` and `models/analytics.go`.
- Contract and CE stub: `services/team_budget/`. Wiring: `Service.InitBudgets` (`services/service.go`). App lifecycle: `services/app_team_budget.go`, `services/app.go`, `services/user_service.go`, `services/group_service.go`.
- Enterprise implementation: `enterprise/features/team_budget/`. Hook in `enterprise/features/budget/service.go`.
- Spend stamping: `analytics/team_stamp.go`. Edge sync: `grpc/budget_sync_service.go`, `microgateway/internal/services/team_blocks.go`, `budget_sync_handler.go`, `budget_service.go`.
- API: `api/team_budget_handlers.go`.

## Follow-ups

- The portal does not yet show the team name, or the team pool, to App owners (it receives `team_id`/`budget_source`).
- SSO profiles cannot yet set a user's budget team from a claim.
- Teams have no team-admin role; only platform roles with groups:write manage team budgets.
- Chat is not enforced against team budgets (nor against App/LLM budgets today).
