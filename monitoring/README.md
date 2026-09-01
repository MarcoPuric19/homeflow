# Local monitoring

This directory contains the local-only HomeFlow observability stack:

- Prometheus 3.13 stores seven days or at most 750 MiB of metrics.
- Grafana OSS 13.2 has a provisioned Prometheus data source and dashboard.
- kube-state-metrics 2.18 exposes Kubernetes 1.34 object state.
- kubelet cAdvisor metrics provide local pod/container CPU and memory usage.

All services are `ClusterIP`. Access is possible only through an explicit
loopback port-forward. Grafana analytics, update checks, news and external
plugins are disabled. No metrics leave the laptop.

kube-state-metrics is deliberately restricted to pods, deployments,
StatefulSets, ReplicaSets, PVCs and nodes. It has no permission to read Secrets,
webhooks or certificate objects.

## Alerting rules

Rules live as the `alerts.yml` key in the same ConfigMap as the scrape
configuration and cover four areas: availability (`up`, 5xx ratio, p95 latency),
database (pool saturation against the five-connection ceiling, error rate, slow
queries), security (rejected logins beyond what a single source can produce
under the rate limit) and household (due expenses, slow receipt uploads).

**There is deliberately no Alertmanager.** It costs roughly 64 MiB of RAM that
this machine does not have spare, and the only recipient is already sitting at
the screen. Alerts are visible under `/alerts` in the Prometheus UI but are not
delivered anywhere. Adding one is a decision about the memory budget recorded in
`Austausch.md`, section 5 — not a free addition.

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

Resource requests total 224 MiB RAM and 80 mCPU. Limits total 768 MiB RAM and
600 mCPU. Two local PVCs request 2 GiB in total.
