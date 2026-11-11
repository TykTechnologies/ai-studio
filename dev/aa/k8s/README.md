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
| `30-mock-llm.yaml` | The benchmark's mock upstream, on the load node |
| `40-load.yaml`, `load.js` | One k6 run as a Job on the load node |
| `aa-k8s.sh` | Secrets, apply, status, the faults and their undo |

Edges are two releases of `helm/charts/microgateway` (`edge-a`, `edge-b`),
rendered with `helm template` and applied with `kubectl`.

Rules: `kubectl` on one context and this namespace only; ClusterIP Services
and `kubectl port-forward`, no LoadBalancer or Ingress; Secrets are created
by `aa-k8s.sh secrets` with random values and are in no file; images come
from the stack's private registry.

Nodes need two labels: `topology.kubernetes.io/zone` (set by the cloud) and
`aa-role` = `site` (one node per zone) or `load` (the node that runs the mock
upstream and k6, so a lost site never takes the load or the upstream away).
