# User-Generated Content (UGC) — Community Resource Submissions

## Overview

The UGC feature enables Portal developers to submit their own Data Sources and Tools (OpenAPI-based APIs) for admin review, classification, and publication into the Portal catalogue. This creates organic, bottom-up growth of the resource ecosystem while maintaining governance through admin oversight.

## Problem

1. **Admin bottleneck** — every new data source or tool requires admin time to onboard
2. **Shadow AI sprawl** — developers share credentials informally, creating ungoverned and duplicate AI data dependencies

## Design Decisions

| Decision | Choice |
|----------|--------|
| Catalogue visibility | Admin assigns during review |
| Contributor trust model | All submissions treated equally |
| Submission scope | Data Sources + Tools (LLMs remain admin-only) |

## Submission States

```
draft → submitted → in_review → approved
                              → rejected
                              → changes_requested → submitted (resubmit)
```

## Data Model

### Submission (`models/submission.go`)

| Field | Type | Description |
|-------|------|-------------|
| `ID` | uint | Primary key |
| `ResourceType` | string | `datasource` or `tool` |
| `ResourceID` | *uint | Set after approval creates the resource |
| `Status` | string | Submission state (see above) |
| `SubmitterID` | uint | FK to users |
| `ReviewerID` | *uint | FK to users (admin who reviewed) |
| `ResourcePayload` | JSON | Full resource configuration for creation on approval |
| `Attestations` | JSON | Array of accepted attestations |
| `SuggestedPrivacy` | int | Submitter's suggested privacy score |
| `PrivacyJustification` | string | Justification for the suggested score |
| `PrimaryContact` | string | Support contact (name + email) |
| `SecondaryContact` | string | Backup contact |
| `SLAExpectation` | string | Expected availability/response time |
| `DataCutoffDate` | *time.Time | Data freshness date (for data sources) |
| `DocumentationURL` | string | External documentation link |
| `Notes` | string | Free text notes |
| `ReviewNotes` | string | Admin-facing internal notes |
| `SubmitterFeedback` | string | Submitter-facing feedback |
| `AssignedCatalogues` | JSON | Catalogue IDs assigned during review |
| `FinalPrivacyScore` | *int | Admin-determined privacy score |
| `SubmittedAt` | *time.Time | When formally submitted |
| `ReviewStartedAt` | *time.Time | When review was claimed |
| `ReviewCompletedAt` | *time.Time | When review was completed |

### AttestationTemplate (`models/attestation_template.go`)

Admin-configurable attestation statements that submitters must accept.

| Field | Type | Description |
|-------|------|-------------|
| `ID` | uint | Primary key |
| `Name` | string | Template name |
| `Text` | string | The attestation text |
| `Required` | bool | Must be accepted to submit |
| `AppliesToType` | string | `datasource`, `tool`, or `all` |
| `Active` | bool | Whether template is active |
| `SortOrder` | int | Display order |

### UGC Fields on Existing Models

**Datasource** — added:
- `CommunitySubmitted` (bool) — true for user-contributed resources
- `SubmissionID` (*uint) — FK to submissions table

**Tool** — added:
- `UserID` (uint) — resource ownership (was missing)
- `CommunitySubmitted` (bool) — true for user-contributed resources
- `SubmissionID` (*uint) — FK to submissions table

## API Endpoints

### Portal Users (authenticated, `/common/`)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/common/submissions` | Create submission (draft or submitted) |
| GET | `/common/submissions` | List own submissions (filtered by status) |
| GET | `/common/submissions/:id` | Get submission detail |
| PATCH | `/common/submissions/:id` | Update draft or resubmit |
| DELETE | `/common/submissions/:id` | Delete draft submission |
| POST | `/common/submissions/:id/submit` | Move from draft to submitted |
| GET | `/common/submissions/attestation-templates` | Get active attestation templates |
| POST | `/common/submissions/validate-spec` | Validate OAS spec (returns structured errors/warnings/extracted ops) |
| POST | `/common/submissions/test-datasource` | Test datasource embedder connectivity |

### Admin (`/api/v1/`, requires admin role)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/submissions` | List all submissions (filterable) |
| GET | `/api/v1/submissions/:id` | Get submission detail |
| POST | `/api/v1/submissions/:id/review` | Claim submission for review |
| POST | `/api/v1/submissions/:id/approve` | Approve with catalogue + privacy score |
| POST | `/api/v1/submissions/:id/reject` | Reject with feedback |
| POST | `/api/v1/submissions/:id/request-changes` | Request changes with feedback |
| POST | `/api/v1/submissions/:id/test` | Test submission connectivity (runs spec validation or embedder test) |

### Attestation Templates (admin)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/attestation-templates` | List templates |
| GET | `/api/v1/attestation-templates/:id` | Get template |
| POST | `/api/v1/attestation-templates` | Create template |
| PATCH | `/api/v1/attestation-templates/:id` | Update template |
| DELETE | `/api/v1/attestation-templates/:id` | Delete template |

## Resource Payload Structure

### For Data Sources

```json
{
  "name": "Customer Vector DB",
  "short_description": "Product embeddings from customer reviews",
  "long_description": "...",
  "icon": "",
  "url": "",
  "db_conn_string": "postgresql://...",
  "db_source_type": "pgvector",
  "db_conn_api_key": "...",
  "db_name": "products",
  "embed_vendor": "openai",
  "embed_url": "https://api.openai.com/v1",
  "embed_api_key": "sk-...",
  "embed_model": "text-embedding-3-small",
  "tags": ["product", "reviews"],
  "active": true
}
```

### For Tools

```json
{
  "name": "Weather API",
  "description": "OpenWeatherMap API for weather data",
  "tool_type": "REST",
  "oas_spec": "<base64-encoded OpenAPI spec>",
  "auth_schema_name": "apiKey",
  "auth_key": "...",
  "available_operations": "getCurrentWeather,getForecast"
}
```

## Approval Flow

1. **Submit** — Submitter fills form, accepts attestations, submits
2. **Admin notification** — Admins are notified of new submission
3. **Review** — Admin claims submission, reviews payload, tests connectivity
4. **Decision** — Admin approves (sets privacy score + catalogues), rejects (with feedback), or requests changes
5. **Resource creation** — On approval, the actual Datasource/Tool is created from the payload
6. **Community badge** — Resource is flagged with `CommunitySubmitted = true`
7. **Submitter notification** — Submitter is notified of the decision

## Update Workflow & Version Tracking

Owners of published community resources can propose changes that go through the same review pipeline:

1. **Owner creates update submission** via `POST /common/submissions/update` with `target_resource_id`
2. Submission has `is_update: true` and follows the same draft → submitted → review → approve/reject flow
3. **On approval**, the system:
   - Snapshots the current resource state into `submission_versions` table
   - Applies the update payload to the existing resource (in-place update, not replacement)
   - Increments the version number
4. **Rollback** — Admin can revert to any previous version via `POST /api/v1/submissions/:id/rollback/:version_id`
   - Before rollback, the current state is also snapshotted (so rollback is itself reversible)

### SubmissionVersion Model

| Field | Type | Description |
|-------|------|-------------|
| `ID` | uint | Primary key |
| `SubmissionID` | uint | The update submission that triggered this snapshot |
| `ResourceID` | uint | The resource that was snapshotted |
| `ResourceType` | string | datasource or tool |
| `VersionNumber` | int | Incrementing version counter per resource |
| `Payload` | JSON | Full snapshot of resource state before the update |
| `ChangedBy` | uint | User who proposed the change |
| `ApprovedBy` | uint | Admin who approved |
| `ChangeNotes` | string | Description of what changed |
| `RolledBackAt` | *time.Time | Set when this version was restored via rollback |
| `RolledBackBy` | *uint | Admin who performed rollback |

### Version/Rollback Endpoints (admin)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/submissions/:id/versions` | List version snapshots for submission's resource |
| POST | `/api/v1/submissions/:id/rollback/:version_id` | Rollback resource to a previous version |

### Update Submission Endpoint (portal)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/common/submissions/update` | Propose changes to an owned published resource |

## Security

- Credentials in `ResourcePayload` are encrypted via the existing secrets management (AES)
- On approval, credentials are transferred to the resource's standard encrypted fields
- All API responses redact credentials (`[redacted]`)
- `ALLOW_INTERNAL_NETWORK_ACCESS` is enforced for connection strings and URLs
- Only submitters can view/edit their own submissions
- Admin review actions require admin role

## Future Phases

### Phase 2 — Robustness (backend complete)
- ~~Credential validation / connection testing on submit + in review~~ (Done)
- ~~Version tracking + rollback for published resource updates~~ (Done)
- ~~Duplicate detection on submission~~ (Done)
- ~~Orphan management when contributors leave~~ (Done — integrated into user deletion flow)
- ~~"Nominate from existing app" shortcut~~ (Done)
- Enhanced resource detail view (frontend only — backend data available)

### Phase 3 — Governance at Scale
- Deprecation/sunset workflow
- Health monitoring + auto-disable for community resources
- Full audit trail
- Bulk review actions
- Usage analytics for contributors
- Breaking change detection for tool spec updates
