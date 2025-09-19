# The Shape of a Full-Stack Plugin

**A plugin = one OCI artifact** with two payloads:

1. **Server runtime** (your current HashiCorp go-plugin binary + protobufs)
2. **UI runtime** (one of: Web Component bundle, Module Federation remote, or iFrame micro-app)
   …plus a **manifest** that declares routes, menu items, permissions, RPC surfaces, and static assets.

```
artifact:
  ├─ /plugin.manifest.json        # required
  ├─ /server/plugin-binary        # required (your current go-plugin)
  ├─ /server/plugin.proto         # required (Call, Describe, Health, etc.)
  ├─ /ui/webc/entry.js            # option A (Web Component bundle as ESM)
  ├─ /ui/mf/remoteEntry.js        # option B (Module Federation remote)
  ├─ /ui/iframe/app.html          # option C (self-contained app)
  └─ /assets/*                    # optional icons, screenshots, CSS, etc.
```

### Example `plugin.manifest.json`

```json
{
  "id": "com.example.audit-trails",
  "version": "1.2.0",
  "name": "Audit Trails",
  "description": "Adds audit trail search and export.",
  "permissions": {
    "kv": ["read", "write", "list"],
    "rpc": ["call"],
    "routes": ["GET:/plugin/com.example.audit-trails/rpc/*"],
    "ui": ["sidebar.register", "route.register"]
  },
  "kvNamespace": "plugin:com.example.audit-trails",
  "rpc": {
    "basePath": "/plugin/com.example.audit-trails/rpc",
    "proto": "server/plugin.proto",
    "entrypoint": "Call"
  },
  "ui": {
    "slots": [
      {
        "slot": "sidebar.section",
        "label": "Audit",
        "icon": "/assets/audit.svg",
        "items": [
          {
            "type": "route",
            "path": "/audit-traillist",
            "title": "Trails",
            "mount": {
              "kind": "webc",
              "tag": "audit-trails-page",
              "entry": "/ui/webc/entry.js",
              "props": { "rpcBase": "/plugin/com.example.audit-trails/rpc" }
            }
          },
          {
            "type": "route",
            "path": "/audit-settings",
            "title": "Settings",
            "mount": {
              "kind": "webc",
              "tag": "audit-settings-page",
              "entry": "/ui/webc/entry.js"
            }
          }
        ]
      }
    ]
  },
  "compat": {
    "app": ">=2.6 <3.0",
    "api": "ui-v1, kv-v1, rpc-v1"
  },
  "security": {
    "csp": "script-src 'self'; object-src 'none'; frame-ancestors 'none'"
  },
  "assets": ["/assets/audit.svg", "/assets/readme.md"]
}
```

---

# Server-Side Contract (stable & boring)

You already do this, but add two universal methods so the UI can introspect and the host can health-check:

**Protobuf**

```proto
service Plugin {
  rpc Describe(DescribeRequest) returns (DescribeResponse);
  rpc Call(CallRequest) returns (CallResponse);  // generic RPC
  rpc Health(HealthRequest) returns (HealthResponse);
}

message DescribeResponse {
  string id = 1;
  string version = 2;
  repeated string capabilities = 3; // ["kv", "ui", "stream", ...]
}
```

**HTTP surface (namespaced & auth’d)**

* `POST /plugin/{id}/rpc/call/{command}` → Plugin.Call
* `GET  /plugin/{id}/health` → Plugin.Health
* Enforce **RBAC scopes** derived from `plugin.manifest.json.permissions`.
* Provide **KV façade** to the plugin via RPC or a tiny sidecar API the host gates:

  * `GET/PUT/DELETE /plugin/{id}/kv/{key}` restricted to `kvNamespace` from manifest.

> This keeps plugins from touching your real DB directly; they get a constrained, namespaced K/V with JSON values.

---

# UI Architecture (the “hard” bit, made safe)

Give plugin authors **three ways** to ship UI, all wired through **the same manifest** and **the same slot system**. You can support all three; pick A as your default.

### A) **Web Components (ESM) – default**

* Framework-agnostic, easy to sandbox with Shadow DOM.
* Load at runtime via dynamic `import()` from a URL you serve (verified from OCI).
* Stable **UI slot API** via a small **TypeScript SDK** (props + events).
* CSP-friendly, no privileged APIs by default.

**Host boot code (React) – simplified**

```ts
// at app bootstrap
const registry = await fetch('/api/plugins/ui-registry').then(r => r.json());

for (const entry of registry) {
  if (entry.mount.kind === 'webc') {
    // Serve from your app (was verified + extracted from OCI)
    const url = `/plugins/assets/${entry.pluginId}${entry.mount.entry}`;
    await import(/* webpackIgnore: true */ url);   // defines custom elements
    router.register(entry.path, () => (
      React.createElement(entry.mount.tag, {
        "data-rpc-base": entry.props?.rpcBase
      })
    ));
    sidebar.add(entry.slot, entry.label, entry.icon, entry.path);
  }
}
```

**Plugin author UX**

* Write a Web Component:

```ts
// /ui/webc/entry.js
class AuditTrailsPage extends HTMLElement {
  connectedCallback() {
    const rpcBase = this.getAttribute('data-rpc-base');
    this.render();
    this.querySelector('#search').addEventListener('click', async () => {
      const q = this.querySelector('#q').value;
      const res = await fetch(`${rpcBase}/call/search`, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({ query: q })
      }).then(r => r.json());
      this.querySelector('#results').textContent = JSON.stringify(res, null, 2);
    });
  }
  render() {
    this.attachShadow({ mode: 'open' }).innerHTML = `
      <style>:host{display:block;padding:16px}</style>
      <div>
        <h2>Audit Trails</h2>
        <input id="q" placeholder="Filter…" />
        <button id="search">Search</button>
        <pre id="results"></pre>
      </div>
    `;
  }
}
customElements.define('audit-trails-page', AuditTrailsPage);
```

* They don’t need your React code. They ship an ESM file exporting a custom element. Done.

**Pros**: light, safe, minimal coupling, hot-loadable.
**Cons**: devs who want tight React integration must code vanilla or use lit.

---

### B) **Module Federation Remote (for React power users)**

* For plugins that want to render React components and consume your design system.
* You expose a tiny **host contract** (shared libs, UI primitives).
* The plugin ships a `remoteEntry.js` and declares a component to mount for a route or panel.

**Manifest (alt mount kind)**

```json
"mount": {
  "kind": "module-federation",
  "remote": "/ui/mf/remoteEntry.js",
  "exposed": "./PluginPage"
}
```

**Host loader (MF runtime at run-time)**

```ts
async function loadMFRemote(remoteUrl: string, scope: string) {
  await __webpack_init_sharing__("default");
  const container = await loadRemoteContainer(remoteUrl, scope); // inject <script>, await container init
  await container.init(__webpack_share_scopes__.default);
  return container.get('./PluginPage').then(factory => factory());
}
```

**Pros**: seamless React, full design-system reuse.
**Cons**: couples to your bundler/runtime; slightly higher risk if versions drift. Use `semver` pins in `compat.app`.

---

### C) **iFrame Micro-app (maximum isolation)**

* For untrusted or very heavy UI.
* You host a static `app.html` from the plugin’s assets; plugin talks to host over `postMessage` (provide a typed bridge).
* Best when you need absolute isolation or plugins bring their own frameworks/CSS.

**Pros**: strongest sandbox.
**Cons**: least integrated DX (navigation, theming), but acceptable for “admin tools”.

---

# Routing & Slots

* **Slots** are stable anchors your app exposes: `sidebar.section`, `settings.panel`, `route:/…`, `header.toolbar`, etc.
* Plugins **declare** menu entries and **route mounts** in the manifest; the frontend loader wires them up at runtime.
* All plugin routes live under `/plugins/{id}/…` to keep SEO, telemetry, and auth simple.

---

# Security, Isolation, and Permissions

* **OCI verification** (you already do): verify signature → extract to plugin store (immutable path like `/var/lib/…/plugins/{id}/{version}/`).
* **Serve UI assets** from your domain under `/plugins/assets/{id}/{version}/…` with strict **CSP** (from manifest + host defaults).
* **Shadow DOM** for Web Components to avoid CSS/DOM bleed.
* **No direct host JS APIs**. The only bridge is:

  * props/attributes,
  * a **typed UI SDK** you expose (e.g., `window.TykUI.v1`) with narrow methods (openToast, openModal, navigate, getAuthToken),
  * or `postMessage` (iframe case).
* **RBAC**: plugins declare scopes; admins approve scopes on install. UI hides items unless user has scope.
* **Network**: UI calls only your **namespaced RPC**. Gate everything by session + CSRF; plugin gets zero direct DB access.
* **Rate limits / circuit-breakers** on `/plugin/{id}/rpc/*`.

---

# Data (KV) Contract

Provide a simple, versioned namespaced K/V with JSON:

* `PUT /plugin/{id}/kv/{key}` `{ "value": any, "etag": "…" }`
* `GET /plugin/{id}/kv/{key}?ifNoneMatch=…`
* `LIST /plugin/{id}/kv?prefix=…&limit=…`
* Optional TTL.

On the server side, implement as a dedicated `plugin_data` table: `(tenant_id, plugin_id, key, value_jsonb, etag, updated_at)`, with row-level tenancy.

---

# Hot-Loading & Lifecycle

* Install: host pulls OCI → verifies → extracts → registers manifest to `/api/plugins/ui-registry`.
* Frontend periodically polls long-ETag or gets a WS push → **dynamically imports** new entries and registers routes; no full reload needed.
* Uninstall/disable: host emits unload event. For Web Components, nothing to do; for MF, unmount React tree; for iframe, remove node.
* Versioning: keep **multiple versions** side-by-side until no route references them, then GC.

---

# Dev Experience (make it delightful)

Ship a **Plugin SDK + CLI**:

```
tyk-plugin init ui --kind webc
tyk-plugin dev --hot          # local dev: serves ui bundle on localhost:5173, proxies /rpc to a local plugin server
tyk-plugin build              # outputs the OCI layout locally
tyk-plugin sign --key <k>     # cosign sign
tyk-plugin push <registry>    # oras push
```

**SDK content**

* **TypeScript types**: `PluginManifest`, `Slot`, `Mount`, `KVClient`, `RpcClient`.
* **UI helpers**:

  * `createWebComponent({ tag, render(ctx) })`
  * `useRpc(base)`, `useKv(namespace)`
  * `useHost()` → theming, toasts, modals, i18n, auth token
* **Scaffolds** for Web Components (vanilla or lit), MF+React, and iFrame.

**Local dev loop**

* CLI boots a dev server that:

  * watches `/ui/*`,
  * serves `/plugin.manifest.json`,
