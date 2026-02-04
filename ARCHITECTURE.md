# AI Studio Microgateway Architecture

This document outlines key architectural principles and patterns to follow when developing the AI Studio microgateway, particularly concerning the interaction between the control plane and edge gateways in a distributed environment.

## Core Principles

The primary goal is to ensure that edge gateways are resilient, secure, and efficient, even when they experience temporary synchronization delays with the control plane.

### 1. Pull-on-Miss for State Synchronization

Instead of relying solely on a push-based mechanism (e.g., broadcasting updates) to synchronize state like Application data, edge gateways should implement a **pull-on-miss** pattern.

**Rationale:**
- **Resilience:** A pure push system can lead to inconsistencies if an edge gateway misses an update due to network issues or being temporarily offline. A pull-on-miss mechanism allows edges to self-heal by fetching the data they need, when they need it.
- **Efficiency:** It avoids broadcasting all changes to all nodes, which can be inefficient in large-scale deployments.

**Implementation:**
- When an edge gateway receives a request with a token it recognizes but for which it does not have the corresponding Application data cached locally, it should not immediately reject the request.
- Instead, it should make a synchronous call to the control plane to fetch the required Application details.
- If the Application is successfully fetched and validated, it should be cached locally (e.g., in the local SQLite database) for subsequent requests.

### 2. Strict Namespace Validation

Security and data isolation between namespaces are critical. When an edge gateway pulls data from the control plane, it **must** validate that the data belongs to a namespace it is configured to manage.

**Implementation:**
- After fetching Application data via the pull-on-miss mechanism, the edge gateway must perform a namespace check.
- The gateway has a configured list of allowed namespaces.
- The validation logic is as follows:
    - If the Application's namespace is `default`, it is always considered valid.
    - Otherwise, the Application's namespace must exist in the edge gateway's list of allowed namespaces.
- If the namespace validation fails, the Application data must be discarded, and the request should be rejected. The invalid data must **not** be cached locally.

### 3. Efficient and Atomic gRPC Communication

To minimize latency and reduce complexity, gRPC communication between the edge and the control plane should be designed to be as efficient as possible. Prefer extending existing RPCs over adding new ones for closely related data.

**Rationale:**
- **Performance:** Reduces the number of network round-trips.
- **Atomicity:** Ensures that related data (like a token and its associated application) are retrieved together, preventing race conditions or intermediate inconsistent states.

**Implementation Example:**
- When implementing the pull-on-miss pattern for Application data, instead of adding a new `GetApplication` gRPC endpoint, the existing `ValidateToken` endpoint should be extended.
- The `TokenValidationResponse` message should be modified to include the full `Application` object.
- This allows the edge gateway to get all the information it needs in a single call, simplifying the client-side logic and improving performance.
