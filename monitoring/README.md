# Local monitoring

This directory contains the local-only HomeFlow observability stack:

- Prometheus 3.13 stores seven days or at most 750 MiB of metrics.
- Grafana OSS 13.2 has a provisioned Prometheus data source and dashboard.
- kube-state-metrics 2.18 exposes Kubernetes 1.34 object state.
- kubelet cAdvisor metrics provide local pod/container CPU and memory usage.
- Grafana itself is scraped, for one reason only: `grafana_plugin_build_info`
  makes the plugin denylist observable instead of merely commented.

All services are `ClusterIP`. Access is possible only through an explicit
loopback port-forward. Grafana analytics, update checks, news and the plugin
background installer are disabled, and twelve unused bundled data-source
plugins are switched off via `GF_PLUGINS_DISABLE_PLUGINS` — each one used to
run as its own process, together about 59 MiB. No metrics leave the laptop.

The namespace has NetworkPolicies of its own. Until then it had none at all,
and Grafana was reaching out to grafana.com on every start. One caveat is
written into the policy file and worth repeating: the egress rule to the API
server is a statement of intent, not a control — this CNI does not filter
traffic to the node's host network.

Prometheus and Grafana are StatefulSets, not Deployments. Each is a single
process holding an exclusive lock on a ReadWriteOnce volume, and a Deployment
starts the replacement before the old pod is gone.

kube-state-metrics is deliberately restricted to pods, deployments,
StatefulSets, ReplicaSets, PVCs and nodes. It has no permission to read Secrets,
webhooks or certificate objects.

## Alerting rules

Rules live as the `alerts.yml` key in the same ConfigMap as the scrape
configuration and cover five areas: availability (`up`, 5xx ratio, p95 latency),
database (pool saturation against the five-connection ceiling, error rate, slow
queries), security (rejected logins beyond what a single source can produce
under the rate limit), household (due expenses, slow receipt uploads) and
resources.

The resource rules compare `container_memory_working_set_bytes` against the
limit reported by kube-state-metrics rather than a number repeated in the rule,
so rule and manifest cannot drift apart silently. They exist because there were
none: Grafana ran at 97 % of its limit and nothing failed until someone went
looking.

One of them says less than its name suggests. `HomeflowClusterFootprintGrown`
measures what the containers use, which is **not** what the laptop uses. The
host memory event of 2026-09-01 came from a browser, an IDE and a Go toolchain,
and cAdvisor sees none of that — evaluated during that event, the expression
reads 14 %. Host pressure is not observable from inside this cluster without a
node_exporter on the machine itself.

**There is deliberately no Alertmanager.** It costs roughly 64 MiB of RAM that
this machine does not have spare, and the only recipient is already sitting at
the screen. Alerts are visible under `/alerts` in the Prometheus UI but are not
delivered anywhere. Adding one is a decision about the memory budget recorded in
`docs/architecture/contracts.md`, section 2 — not a free addition.

Check and reload rules:

```bash
kubectl -n homeflow-monitoring exec deploy/prometheus -- \
  promtool check config /etc/prometheus/prometheus.yml
kubectl -n homeflow-monitoring rollout restart deploy/prometheus
```

The checked-in configuration was validated on the running stack with
`promtool`: one rule file and all ten rules passed. Prometheus reported the
`homeflow-api`, `kube-state-metrics`, `kubernetes-cadvisor` and `prometheus`
targets as healthy.

Metric names are a contract between `backend/internal/observability/metrics.go`,
the Grafana dashboards and these rules. Renaming one empties a panel or disables
an alert without anything failing.

The setup creates one random Grafana password in a Kubernetes Secret only when
none exists. It never prints the password. Use `make grafana-password` when you
intentionally want to display it locally.

Resource requests total 384 MiB RAM and 80 mCPU. Limits total 768 MiB RAM and
600 mCPU. Two local PVCs request 2 GiB in total.

The requests are above measured usage on purpose, not padding. The kubelet on
this cluster reserves nothing and has memory eviction switched off, so the host
kernel decides who dies, by `oom_score_adj` — which for Burstable pods is
`1000 − 1000 × request / capacity`. A request below real usage moves the pod to
the front of that queue. Measured peaks and the full derivation are in
`docs/operations/resource-budget.md`.
