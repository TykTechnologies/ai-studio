# Tyk AI Microgateway - Port Reference

| Port | Protocol | Purpose                         | Default |
|------|----------|---------------------------------|---------|
| 8080 | HTTP(S)  | REST API / LLM Proxy            | Yes     |
| 9090 | gRPC     | Control/Edge communication      | When GATEWAY_MODE=control |

## Firewall Rules

### firewalld (RHEL/CentOS/Amazon Linux)

```
firewall-cmd --permanent --add-port=8080/tcp
firewall-cmd --permanent --add-port=9090/tcp
firewall-cmd --reload
```

### ufw (Ubuntu/Debian)

```
ufw allow 8080/tcp
ufw allow 9090/tcp
```

## Notes

- Port 8080 handles all LLM proxy traffic and the management API.
- Port 9090 is used for gRPC communication when running in `control` mode.
  In `edge` mode, the microgateway connects *outbound* to the control plane
  (AI Studio) and does not listen on a gRPC port.
- All ports are configurable via `/etc/default/tyk-microgateway`.
