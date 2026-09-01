# Resource budget: measured, not estimated

Research report for ticket HF-0086. It reports what the cluster actually uses,
explains what kills containers on this machine, and lists options with their
costs. **It changes no limit, no request and no number in
`docs/architecture/contracts.md`.** Those are Marco's decisions; this document
exists so that they can be made from measurements instead of guesses.

Measured on 2026-09-01 against the `kind-dev` cluster.

## Method

`scripts/resource-report.sh` reproduces every table below. It opens a loopback
port-forward to Prometheus, reads container peaks from the cAdvisor scrape,
joins them against the limits the cluster actually has (not the manifests), and
cleans up after itself. It reads only.

The window is 7 days, which is the full Prometheus retention (contracts.md 2.2).
A larger window returns the same numbers, not older ones.

## What the machine has

| | Value |
| --- | --- |
| Host | Intel i5-7440HQ, 4 cores, 7285 MiB RAM |
| Node `capacity.memory` | 7285 MiB |
| Node `allocatable.memory` | 7285 MiB |
| Reserved for kubelet and the system | **0 MiB** |
| Host actually available during the measurement | 2500–3000 MiB |

The kind node is a container without a memory limit, so it sees the host's full
RAM and reports all of it as schedulable. Kubernetes believes it has 7.1 GiB for
pods. It does not: the desktop, the browser, the IDE, Docker itself and the Go
and npm toolchains take the difference, and they are not visible to the
scheduler.

## Memory: peak against limit

| Container | Peak | Limit | Share | QoS |
| --- | --- | --- | --- | --- |
| grafana | 248.6 MiB | 256 MiB | **97 %** | Burstable |
| homeflow-api | 142.3 MiB | 192 MiB | 74 % | Burstable |
| kindnet-cni | 33.8 MiB | 50 MiB | 68 % | Guaranteed |
| kube-state-metrics | 63.1 MiB | 128 MiB | 49 % | Burstable |
| coredns | 66.9 MiB | 170 MiB | 39 % | Burstable |
| prometheus | 133.3 MiB | 384 MiB | 35 % | Burstable |
| postgres | 47.3 MiB | 384 MiB | 12 % | Burstable |
| kube-apiserver | 292.7 MiB | none | — | Burstable |
| kube-controller-manager | 121.0 MiB | none | — | Burstable |
| etcd | 84.9 MiB | none | — | Burstable |
| kube-scheduler / kube-proxy | 63.0 / 56.1 MiB | none | — | Burstable / BestEffort |
| local-path-provisioner | 37.3 MiB | none | — | BestEffort |

Sum of peaks: 1408 MiB. CPU is not a constraint anywhere: the busiest container
is kube-apiserver at 216 mCPU of 4000, and the HomeFlow API peaks at 8 mCPU of
its 200 mCPU limit.

## Findings

### 1. The API dies at two logins. This was reproduced, not inferred.

`argonMemory = 64 MiB`, `argonParallelism = 2`, and `auth.PostgresService`
holds `passwordSlots: make(chan struct{}, 2)` — two password checks may run at
once, each allocating a 64 MiB Argon2id buffer. The container limit is 192 MiB.

Four deliberately failed logins against the running API, in two concurrent
pairs about two minutes apart, drove the container's cgroup `memory.current`
from 17 MiB to 191.8 MiB of a 192 MiB `memory.max`, and then over it. The
kernel log is unambiguous:

    Memory cgroup out of memory: Killed process (homeflow)
    total-vm:1537660kB, anon-rss:195416kB, oom_score_adj:996
    oom-kill:constraint=CONSTRAINT_MEMCG

`CONSTRAINT_MEMCG` means the container's own limit killed it, not host
pressure. The pod restarted and recovered on its own.

Two things matter here. The 74 % "peak" in the table above is not a load peak —
it is what a single login costs; the historical maximum was almost certainly a
login, not traffic. And the Argon2 buffers are anonymous memory, so nothing can
be reclaimed under pressure: the only outcome is the kill. Go also does not
return freed heap to the operating system immediately, so logins in quick
succession accumulate toward the ceiling rather than each starting from idle.

The exposure is bounded by the threat model — the API is reachable on loopback
only, and this is a two-person household — but two people logging in at the same
moment is ordinary behaviour, not an attack.

`contracts.md` 2.1 already states that the Argon2 parameters, the parallelism
and the memory limit are three numbers that belong together. The measurement
says the current triple does not hold with any margin.

### 2. Nothing reserves memory for the host, and nothing evicts

The kubelet configuration in the kind node has no `kubeReserved` and no
`systemReserved`, and its `evictionHard` contains only disk thresholds:

    evictionHard:
      imagefs.available: 0%
      nodefs.available: 0%
      nodefs.inodesFree: 0%

`memory.available` is absent, so kubelet's memory eviction is switched off
entirely. `MemoryPressure` on the node can therefore essentially never become
true, and Kubernetes will never evict a pod to protect the machine. When RAM
runs out, the host kernel's OOM killer acts instead — and it does not know about
QoS classes, priorities or workloads.

### 3. Six containers died in the same eight minutes. That was the host.

| Container | OOMKilled at |
| --- | --- |
| coredns (×2) | 14:34:57–58 UTC |
| prometheus | 14:34:58 UTC |
| local-path-provisioner | 14:42:39 UTC |
| nginx (namespace `default`) | 14:42:41 UTC |
| kube-state-metrics | 14:42:41 UTC |

Two of these — `local-path-provisioner` and `nginx` — have no memory limit at
all and are `BestEffort`, so a cgroup OOM is impossible for them by
construction. The others peaked far below their limits (coredns 67 of 170 MiB,
prometheus 133 of 384 MiB). Five namespaces failing inside eight minutes is one
event, not five coincidences: the laptop ran out of memory and the kernel picked
victims.

It picked them in a predictable order. Measured `oom_score_adj` on the node:

| Container | `oom_score_adj` | Memory request | QoS |
| --- | --- | --- | --- |
| nginx, local-path-provisioner | **1000** | none | BestEffort |
| kube-state-metrics, homeflow-api | 996 | 32 MiB | Burstable |
| grafana | 992 | 64 MiB | Burstable |
| coredns | 991 | 70 MiB | Burstable |
| prometheus | 983 | 128 MiB | Burstable |

For Burstable pods the kubelet sets `oom_score_adj = 1000 − 1000 × request /
capacity`, which reproduces every value above exactly. The consequence is the
practical lever on this machine:

> **A memory request buys protection from the host kernel. A memory limit only
> buys a kill.**

Requests are currently set far below real usage — the API requests 32 MiB and
uses 142 MiB — which makes the two components we care about most among the
first candidates the kernel considers. Raising requests toward measured usage
costs nothing while the total stays under `allocatable`, and it reorders the
kill list in our favour. `Guaranteed` (request = limit) would give −997 and
effectively remove a pod from the kernel's list; only `kindnet-cni` is
`Guaranteed` today.

### 4. Postgres is not over-provisioned, it is mis-provisioned

The 12 % reading is misleading. No Postgres parameters are set anywhere — the
manifest passes none, so the image defaults apply. Measured from the running
instance:

| Setting | Value | Comment |
| --- | --- | --- |
| `shared_buffers` | 128 MiB | one third of the 384 MiB limit, allocated lazily |
| `effective_cache_size` | 4 GiB | a claim about a machine that does not exist |
| `max_connections` | 100 | the application pool is capped at 5 |
| `work_mem` | 4 MiB | per sort, per connection |

`max_connections = 100` is a promise the 384 MiB limit cannot keep: at roughly
2–5 MiB per backend that alone is 200–500 MiB before `shared_buffers`. It never
shows up today because two users produce almost no concurrency. It is a
landmine for F2 (HF-0002), not spare capacity to reclaim now.

### 5. Grafana runs 7 MiB under its limit, and 12 of its 13 plugins are unused

The main process uses 164 MiB. Alongside it run 13 `gpx_grafana-*` plugin
processes — Elasticsearch, Loki, MySQL, MSSQL, InfluxDB, CloudWatch, Jaeger,
Tempo, Zipkin, Pyroscope, OpenTSDB, PostgreSQL and Prometheus — totalling about
59 MiB. Exactly one of them, Prometheus, is used by a data source.

Grafana 13.2.0 ships a supported switch for this. Its own `defaults.ini` says
of `[plugins] disable_plugins`: a comma-separated list of plugin identifiers to
avoid loading, *including core plugins*.

Measured with two isolated containers of the same image, each capped at 320 MiB
and run without network:

| Run | Plugin processes | Memory |
| --- | --- | --- |
| default | 13 | 229.2 MiB |
| `GF_PLUGINS_DISABLE_PLUGINS=…` (Prometheus kept) | 1 | 170.1 MiB |

**59 MiB saved, 26 %.** The reduced run produced fewer error lines than the
reference, not more; all errors in both runs were failed DNS lookups caused by
`--network none`, none of them caused by a disabled plugin.

This alone takes Grafana from 97 % to about 74 % of its current limit, without
touching the limit and without losing a feature we use.

### 6. Three pods in namespace `default` belong to nobody

`local-web`, `webtest` and an `nginx` are running. None appear in any manifest
in this repository. Together they peak at about 18 MiB, all three are
`BestEffort` with `oom_score_adj` 1000, and the `nginx` has already been
OOMKilled four times. They are noise rather than a burden — but they are
unexplained, and they are first in line for the kernel. Not removed: their
origin is unknown, and deleting somebody's pod is not a research task.

### 7. `kubectl top` does not work

There is no metrics-server in the cluster. Every measurement here required a
port-forward to Prometheus, which is why `scripts/resource-report.sh` exists.
A metrics-server would cost roughly 50–70 MiB of a budget that finding 3 shows
is already exhausted. That is a decision, not an obvious improvement.

## Options, with costs

Ordered by measured urgency. None of these are applied.

### API, 192 MiB — finding 1. Pick one; doing nothing is also a choice.

| Option | Cost | Effect |
| --- | --- | --- |
| Raise the limit to 320 MiB | +128 MiB of a budget that is already tight | Two concurrent logins fit with margin. Does not change that a third would not. |
| Reduce `passwordSlots` to 1 | Logins serialise; the second waits roughly 100–200 ms | Argon2 peak halves to 64 MiB. Cheapest fix. Weakens nothing cryptographically. |
| Lower `argonMemory` to 32 MiB | Halves the cost of a brute-force attempt against a stolen hash | Peak halves. This is a security parameter — ADR territory, not a tuning knob. |
| Raise the request from 32 MiB toward 142 MiB | None while the total stays under `allocatable` | Does not prevent the cgroup kill, but moves the API far down the host kernel's list. |

The first two are independent and can be combined. The third should not be
chosen for memory reasons alone.

### Grafana, 256 MiB — finding 5

Set `GF_PLUGINS_DISABLE_PLUGINS` to everything except the Prometheus data
source. Costs nothing, measured at −59 MiB, and leaves 26 % headroom instead of
3 %. If a data source is added later, the list has to be amended — that is the
only maintenance burden, and it is one line.

### Node — findings 2 and 3

| Option | Cost | Effect |
| --- | --- | --- |
| Set `kubeReserved` and `systemReserved` in the kind config | `allocatable` drops by the reserved amount | The scheduler stops believing the laptop is empty. Honest, but this cluster is not scheduling-constrained today, so the benefit is mostly future. |
| Add `evictionHard: memory.available` | Kubelet starts evicting under pressure | Turns a kernel kill of a random victim into a kubelet eviction of the lowest-QoS pod. Requires the requests from finding 3 to be sensible first, otherwise it evicts the wrong things. |
| Raise requests toward measured usage | None, while the sum stays under `allocatable` | Reorders the kernel's kill list. Cheapest of the three and a precondition for the second. |
| Give the kind node container a memory limit | The node OOMs as a unit instead of the host thrashing | Protects the desktop from the cluster, at the price of a harder failure mode inside it. |

### Postgres, 384 MiB — finding 4

Do not reduce the limit. Set the parameters instead, so that the limit means
something: `max_connections` in the region of the actual pool rather than 100,
`effective_cache_size` to something this machine could plausibly have,
`shared_buffers` chosen deliberately. Costs one ConfigMap and a restart. Until
F2 (HF-0002) has a load profile, any number chosen for 500 concurrent users is
a guess — but 100 connections against a 5-connection pool is a guess that is
already wrong.

## What was not measured

- **Load.** Everything here is a two-user idle system plus deliberate probes.
  Nothing in this report tells you what 500 concurrent users cost (HF-0002).
  Peaks under load will be different in kind, not just in size.
- **The OCR pod (HF-0076) has no budget at all.** It does not exist yet, so it
  could not be measured. Finding 3 says there is no headroom to give it without
  taking it from something else.
- **A second cluster for Mission Control (MC-0004)** would add roughly 1.03 GiB
  to a machine that already OOM-kills under its current load. That decision now
  has a measurement behind it.
- **Whether the Grafana saving survives real dashboards.** It was measured on a
  fresh instance without our dashboards or data sources provisioned.
- **Whether `evictionHard` behaves as expected in kind.** Not tested; kind
  disables it deliberately and the reason for that is not documented upstream in
  a way this report verified.
- **The exact trigger in finding 1** — whether it takes two concurrent logins or
  four in quick succession — was not isolated. Doing so needs another run
  against the live API, and one OOMKill of Marco's running system was enough.

---

## What was implemented (HF-0090, 2026-09-01)

This section is the record of which options above were taken. It does not
replace the findings; it says what happened to them.

| Finding | Action | Verified |
| --- | --- | --- |
| 1 — API dies at two logins | `passwordConcurrency` 2 → 1 (HF-0087, option A) | Guard test; the request raise and contracts.md wait on HF-0071 |
| 3 — requests protect, limits kill | grafana 64 → 160 MiB, prometheus 128 → 160 MiB, kube-state-metrics 32 → 64 MiB | applied, all pods running |
| 4 — Postgres mis-provisioned | `max_connections=20`, `shared_buffers=96MB`, `effective_cache_size=192MB`, `maintenance_work_mem=32MB` | throwaway container; **not applied to the live database** |
| 5 — Grafana at 97 % | `GF_PLUGINS_DISABLE_PLUGINS` for the 12 unused core plugins | applied and measured, see below |
| 2 — node reserves nothing | `kubernetes/kind-cluster.yaml` written | **not effective** until the cluster is rebuilt |
| 6 — unowned pods | — | blocked, HF-0089 |
| 7 — no metrics-server | — | not done; costs RAM, Marco's decision |

### Grafana, measured after the change

Comparing like with like — steady state against steady state, from the
`container_memory_working_set_bytes` series across the restart:

    plugin processes    13 -> 1           (59.3 MiB -> 17.0 MiB RSS)
    steady state     214.8 -> 164.8 MiB   84 % -> 64 % of the 256 MiB limit
    anon                     134.7 MiB    the part that can actually kill
    memory.events oom_kill         0      no pressure since the restart

Two numbers here are easy to misread, so they are stated plainly. The old pod's
**historical peak** was 248.6 MiB (97 %) and its steady state 214.8 MiB (84 %);
the honest comparison is the second pair, and it is a 50 MiB saving that matches
the 59 MiB measured on isolated containers.

And the new pod still spikes to 232.9 MiB (91 %) for about a minute at startup
before settling. `memory.peak` reads 255 MiB of 256 for the same reason. That
looks alarming and is not: 54.8 MiB of it is inactive page cache, which the
kernel reclaims under pressure instead of killing, and `memory.events` reports
`oom_kill 0` and `max 0` — the cgroup was never under real pressure. `anon` is
the number that decides an OOM, and at 134.7 MiB it has room.

Note for the next measurement: `scripts/resource-report.sh` defaults to the full
7-day window, so it keeps reporting the pre-change peak until that ages out.
`WINDOW=15m ./scripts/resource-report.sh` shows the current state.

### A defect found while applying this

Neither the Prometheus nor the Grafana Deployment declared an update strategy,
so both used the default `RollingUpdate`. Each has one replica writing to a
single ReadWriteOnce volume with an exclusive lock. The new Prometheus pod
therefore started while the old one still held the TSDB and died with

    opening storage failed: lock DB directory: resource temporarily unavailable

five times, while the old pod kept serving. Grafana survived the same race by
luck. This was latent, not introduced here — it needed the first-ever update of
these Deployments to surface, and this was it.

`strategy: { type: Recreate }` fixed it, and HF-0092 replaced that with the
object type that carries the same guarantee without anyone having to remember a
field: both are now StatefulSets, like postgres already was. The existing
`prometheus-data` and `grafana-data` claims are mounted as ordinary volumes
rather than through `volumeClaimTemplates`, which would have created new PVCs
and abandoned the metrics history this whole report is built on. Verified: the
TSDB still answers queries from twelve hours before the migration.

### Not applied on purpose

The Postgres parameters change the running household database and require a
restart. They are verified in a throwaway container of the same image, and left
for the deploy pipeline rather than applied by hand — unlike the NetworkPolicies
in HF-0088, they need no cluster to be trusted.

`kubernetes/kind-cluster.yaml` takes effect only on the next
`kind create cluster --config kubernetes/kind-cluster.yaml`. The running node
still reports `allocatable` = `capacity` and still has memory eviction switched
off. Rebuilding the cluster is Marco's call, and `systemReserved` is a budget
number in the sense of contracts.md section 2 — the file names the arithmetic so
that the number can be argued with.

## Follow-up from the audit (HF-0091, HF-0092)

Asked whether the above was best practice, four things did not survive the
question.

**There were no memory alerts at all.** Ten alert rules, none on
`container_memory`. Grafana sat at 97 % of its limit and nothing failed —
the only reason anyone noticed is that HF-0086 went looking. Instrumentation
belongs before the numbers, not after them. Four rules now exist, and the first
was checked against the recorded history rather than only for syntax: evaluated
over the last fourteen hours it produces 26 breaches for grafana with a peak of
93.6 %, so it would have reported the very condition this report had to be
written to find.

**One of those alerts was wrong and was rewritten.** A rule summing container
memory against machine memory was meant to catch the host-pressure event of
2026-09-01. Evaluated at 14:40 UTC, in the middle of that event, it reads
14.4 % — nowhere near its threshold. The pressure came from the browser, the
IDE and the toolchains, and cAdvisor sees none of that. **From inside the
cluster, host memory pressure is not observable.** Catching it needs a
node_exporter on the laptop itself, which is a new permanent component and
therefore a decision, not an obvious fix. The rule now states what it can
actually see and says plainly what it cannot.

**The password semaphore was in the wrong place.** It wrapped all of `Login`,
database query included, which was invisible at a capacity of two and
serialised every login at a capacity of one. It now wraps only the Argon2 call,
in both the known-user and unknown-user paths.

**The NetworkPolicy egress to the API server is not enforced, and now says so.**
It allowed two CIDRs on two ports because the correct one had not been measured.
Narrowing it to `172.18.0.0/16:6443` keeps the cAdvisor scrape working — but
removing the rule entirely also keeps it working. A probe pod covered only by
`default-deny` reaches `172.18.0.3:6443` and `10.96.0.1:443`, while it cannot
reach another pod at `10.244.0.18:9090`. kube-network-policies filters the
pod-to-pod datapath; destinations in the node's host network bypass it, the same
way port-forward and kubelet probes do in the other direction. The rule stays as
a statement of intent and would take effect under Calico or Cilium, but it is
not a control. The gap itself is HF-0093.
