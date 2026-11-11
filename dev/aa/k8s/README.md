# The two-site stack on Kubernetes (failover under load)

Draft for bead midsommar-clean-4x5y.29. The compose stack in `dev/aa` on a
cluster, with a site being an availability zone, so that failover can be
tested under sustained load. Nothing here is a supported way to deploy AI
Studio: PostgreSQL has no failover manager other than `aa-k8s.sh`.

| File | What |
|---|---|
| `00-namespace.yaml` | Namespace `aa-load` |
| `05-pg-roles.yaml` | ConfigMap naming the primary: the failover state |
| `10-postgres.yaml`, `pg-node-k8s.sh` | `pg-a` (zone a) and `pg-b` (zone b), synchronous replication, password on every TCP connection |
| `20-studio.yaml` | `studio-a`, `studio-b`, and a cuttable gRPC proxy in front of each (`cp-a`, `cp-b`) |
| `25-edges-service.yaml` | One address for both edges: the load balancer's part |
| `30-tools.yaml` | The gateway benchmark's tools (`benchmarks/gateway`): `mockllm`, two `gwbench` generators (short requests, streams), and a small loop for admin writes and logins |
| `scenarios/` | Constant-rate and ramp scenarios for `gwbench`, at the benchmark's levels |
| `adminloop.py` | What `gwbench` does not do: admin writes and logins through each Studio |
| `aa-k8s.sh` | Secrets, apply, status, the faults and their undo |

Edges are two releases of `helm/charts/microgateway` (`edge-a`, `edge-b`),
rendered with `helm template` and applied with `kubectl`.

Rules: `kubectl` on one context and this namespace only; ClusterIP Services
and `kubectl port-forward`, no LoadBalancer or Ingress; Secrets are created
by `aa-k8s.sh secrets` with random values and are in no file; images come
from the stack's private registry.

Nodes carry `aa-load/role` = `hub`, `edge` or `tools`, `aa-load/site` = `a`
or `b`, and the taint `aa-load=<role>:NoSchedule`; every pod here selects its
role (and site) and tolerates the taint. A hub node holds its site's Studio,
PostgreSQL node and control proxy; a tools node holds the mock and the load
generators, outside both sites, so a lost site never takes the load away.

Load levels and the validity rule are the gateway benchmark's
(`benchmarks/gateway/benchmark-results.md`): the total through both edges is
one edge's figure, so a lost site leaves the survivor at the benchmarked
level. Run results go to `results/` here, which is not committed.
