# Tyk AI Studio - Port Reference

| Port  | Protocol | Purpose                          | Default |
|-------|----------|----------------------------------|---------|
| 8080  | HTTP(S)  | Web UI / REST API                | Yes     |
| 8989  | HTTP     | Documentation Server             | Yes     |
| 9090  | HTTP(S)  | Embedded Gateway (LLM proxy)     | Yes     |
| 50051 | gRPC     | Control Plane (hub-spoke comms)  | When GATEWAY_MODE=control |

## Firewall Rules

### firewalld (RHEL/CentOS/Amazon Linux)

```
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --permanent --add-port=8989/tcp
firewall-cmd --permanent --add-port=9090/tcp
firewall-cmd --permanent --add-port=50051/tcp
firewall-cmd --reload
```

### ufw (Ubuntu/Debian)

```
ufw allow 8080/tcp
ufw allow 8989/tcp
ufw allow 9090/tcp
ufw allow 50051/tcp
```

## Notes

- Port 8080 serves both the admin UI (frontend) and the REST API.
- Port 9090 is the embedded gateway that proxies LLM requests.
- Port 50051 is only needed when running in `control` mode (hub for edge gateways).
- All ports are configurable via `/etc/default/tyk-ai-studio`.
