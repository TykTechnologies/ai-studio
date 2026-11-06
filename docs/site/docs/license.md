# License and Quotas (Enterprise)

An Enterprise licence limits how much of AI Studio a deployment may use. This page explains what is limited, what happens near and at a limit, where an Owner sees the licence, and the per-user App quota administrators set themselves.

## What the licence limits

| Limit | What uses it |
|-------|--------------|
| **Edge namespaces** | Each edge namespace other than the default one. See [Licensed Namespaces](./edge-gateways.md#licensed-namespaces-enterprise). |
| **Users with admin access** | Each enabled user who can open the admin area, whatever their role: Viewers and Auditors count, and so does access through a team's role. |
| **Users with portal access** | Each enabled user with portal access. |
| **Apps** | Each App, however it was created. |

A limit is a number or unlimited. Disabling a user frees their seats; deleting an App frees its slot.

## At a limit

A change that would go over a limit is refused with a message that says what to do, for example:

> Your organisation has reached its licensed limit of 10 apps. Remove an app to make room, or contact your administrator to increase the quota.

The API answers HTTP 409 with `errors[0].code` set to `QUOTA_EXCEEDED`; plugins calling through the SDK get gRPC `ResourceExhausted`. This applies to creating Apps, creating and editing users, adding users to teams, and granting roles, including a role granted to a team, which gives every member admin access.

Nothing that exists is taken away. If a new licence has lower limits than what is in use, everything keeps working, and new use is refused until usage drops below the limit.

**Single sign-on and host sign-in** never fail because of a limit. A user who signs in through SSO (or a host application) and whose new access would go over a limit signs in without that access: no portal access, or not added to the team that would give admin access. The Owners get a notification saying who was left out and why, and an administrator can grant the access once a seat is free.

## Near-limit notices

When a limit is close (one short of its maximum, or at it), a banner appears at the top of the admin area for users with admin access, naming the limit and its usage. Portal users see a banner when they are close to their own App quota.

Each banner can be dismissed with its close button, and stays dismissed for that user. A licence with a different number for that limit shows the banner again.

## The License page

**Admin > Settings > License** is visible to **Owners** only (users holding the Owner role; without roles, the first administrator). Other administrators do not see it in the menu, the page shows that it is for Owners, and the API (`GET /api/v1/license`) answers 403.

The page shows:

- the products the licence covers and when it expires, with the days left;
- each limit with its usage ("3 of 10", or "of unlimited"), marked **At limit** when it is reached;
- the licensed features;
- the **licensed namespaces**, each with when it took its slot, its connected edge gateways, and, for an idle one, when its slot frees itself. **Release slot** frees a slot now, for a namespace with no connected edge gateway (`DELETE /api/v1/license/namespaces/{namespace}`); the freed slot goes to the next waiting namespace;
- the **waiting namespaces**, in the order they will get a slot.

## App quota

Administrators can limit how many Apps each user creates for themselves, through the AI Portal or a plugin. Apps an administrator creates in the admin area do not count against it; they count only against the licence's App limit.

- **The global default** is set with the environment variable `DEFAULT_APP_QUOTA`: a number, or `*` for unlimited. When it is not set, each user may create **1** App.
- **A team's quota** overrides the default for its members: open the team (**Admin > Access > Teams**), and in **App quota** set **Apps per member**, or choose **Unlimited**. Removing a team's quota returns its members to the default. The API is `GET`, `PUT` and `DELETE /api/v1/groups/{id}/app-quota`.
- **A user in several teams** gets the most generous quota among their teams that set one, and the default only if none does.
- The quota counts the Apps the user owns. The AI Portal's App builder shows "You are using 1 of 3 apps you may create", and creating one more is refused with the same 409 `QUOTA_EXCEEDED` message. A user's quota is at `GET /common/me/app-quota`.

Community Edition has no App quota.

## See also

- [Edge Gateways](./edge-gateways.md): licensed namespaces and pushes
- [User Management & RBAC](./user-management.md): roles, the Owner role and teams
