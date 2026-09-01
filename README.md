# Local monitoring

This directory contains the local-only HomeFlow observability stack:

- Prometheus 3.13 stores seven days or at most 750 MiB of metrics.
- Grafana OSS 13.2 has a provisioned Prometheus data source and dashboard.
- kube-state-metrics 2.18 exposes Kubernetes 1.34 object state.
- kubelet cAdvisor metrics provide local pod/container CPU and memory usage.

All services are `ClusterIP`. Access is possible only through an explicit
loopback port-forward. Grafana analytics, update checks, news and external
plugins are disabled. No metrics leave the laptop.

The setup creates one random Grafana password in a Kubernetes Secret only when
none exists. It never prints the password. Use `make grafana-password` when you
intentionally want to display it locally.

Resource requests total 224 MiB RAM and 80 mCPU. Limits total 768 MiB RAM and
600 mCPU. Two local PVCs request 2 GiB in total.
