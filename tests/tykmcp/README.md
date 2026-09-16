# Tyk Dashboard MCP integration: live verification

`live_e2e.py` drives the whole feature against a real Tyk Dashboard and Gateway
and an enterprise AI Studio, following section 16 of
`features/TykMCPIntegration.md`: connection and probe, registration, policy
creator, publishing, App binding, key minting and gateway calls, drift
(narrowing, widening approval, external policies), missing and resumed
proxies, keys deleted on the Dashboard, rotation, catalogue-mode handoff with
webhook delivery, manual creation and linking, gateway tags. It cleans up
after itself and never prints a key.

Last run 2026-09-16 against Dashboard 5.14 + Gateway EE: 80 checks passed.

1. Run the demo MCP server on the host so the Gateway container can reach it:
   `cd mcpsrv && GOFLAGS=-mod=mod go run . 4020 > mcpsrv.log 2>&1 &`
2. Run the webhook receiver: `python3 hookrecv.py 4021 &`
3. Export the variables listed at the top of `live_e2e.py` (a Dashboard user
   access key, an AI Studio administrator key) and run `python3 live_e2e.py`.

The dev stack needs `dev/.env.secrets` with `TYK_AI_LICENSE` and
`WEBHOOKS_ALLOW_INTERNAL_TARGETS=true`, and
`FRONTEND_PORT=3100 STUDIO_PORT=8090 make dev-start-ent` when a Dashboard
already listens on 3000. The Studio container only rebuilds its binary on
`make dev-restart-studio`.
