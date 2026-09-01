# HomeFlow architecture

## Context

HomeFlow is a private, API-first household platform for exactly two
administratively managed users. It runs locally and must remain useful without
any external integration or internet connection.

## Version 1 shape

```text
future web/mobile client
          |
          v
  private access boundary
          |
          v
    HomeFlow HTTP API  ---- local receipt filesystem
          |
          +------------ PostgreSQL
          |
          +------------ in-process worker/event handlers
          |
          +------------ optional connector ports -> mocks by default
```

The code is a modular monolith. Domain packages own their models and ports;
transport and persistence are adapters. Background jobs initially run in the
same deployable process but remain behind worker interfaces.

## Modules

- `auth`: identities, authentication boundary and request principals
- `shopping`: lists and items; first implemented use case
- `calendar`: events and RFC 5545 recurrence/export concepts
- `expenses`: planned and actual household expenses
- `receipts`: metadata and safe local file references
- `pets`: household-scoped pet master data and the future medical aggregate root
- `integrations`: provider-neutral connector contracts and mocks

Domain events are in-process messages in version 1. An outbox table is included
in the database design so reliable asynchronous delivery can be added without
introducing Kafka or another broker.

## Data and privacy boundaries

- PostgreSQL is the authoritative store for structured data.
- Receipt bytes remain in non-public local storage; the database stores metadata
  and opaque storage keys only.
- Logs must not contain credentials, tokens, receipt content or private notes.
- External connectors are disabled unless explicitly configured.

## Sprint 3 runtime boundary

PostgreSQL is now authoritative for users, sessions, audit events and shopping
data. Embedded migrations run under a PostgreSQL advisory lock at startup. The
connection pool is capped at five connections. Expenses, monthly summaries and
receipt metadata now use PostgreSQL; receipt bytes use a private local
filesystem/PVC adapter. Calendar and pet master data are exposed by the API and
the same-origin web client.

## Tenant-ready local model

Version 0.6 introduces `households` and `household_members` as the boundary for
all new pet data. The two local accounts are assigned to one default household.
Every pet query joins that membership and returns 404 for a pet in another
household. This keeps the local 0-euro/two-user operating model unchanged while
avoiding a later rewrite of the pet domain. Existing shopping, calendar,
expense and receipt tables are not yet fully tenant-scoped; the repository must
not be described as generally multi-tenant until that separate migration and
its isolation tests are complete.

## Sprint 4 web delivery

The React/TypeScript client is compiled to static files and shipped inside the
same HomeFlow image. The Go process serves the SPA and API on one origin. This
avoids a second runtime container, CORS configuration and another reverse proxy
on the resource-constrained laptop. Vite's development server only exists for
local frontend development and proxies API calls to loopback.

Mobile clients keep using opaque bearer tokens. Browsers use separate endpoints
that place access and rotating refresh tokens in HttpOnly, SameSite=Strict
cookies. Unsafe cookie-authenticated requests additionally require a matching
double-submit CSRF token. Local HTTP requires `HOMEFLOW_COOKIE_SECURE=false`;
the setting must become `true` behind approved HTTPS before any remote access.

## Sprint 5 observability

The API exposes a Prometheus registry on `/metrics` from the same listener as
the API and the SPA. The endpoint is deliberately unauthenticated: it carries
no private labels or values, and the deployment keeps it reachable only inside
the cluster. That trade-off is recorded in the threat model and must be revisited
before any remote access.

Prometheus, Grafana and kube-state-metrics run in a separate `homeflow-monitoring`
namespace, all `ClusterIP`, reachable only through an explicit loopback
port-forward. Alerting rules live in the same ConfigMap as the scrape
configuration. There is no Alertmanager: on a single laptop its memory cost is
not justified while the operator is also the only recipient. Alerts are therefore
visible, not delivered.

Route labels come from Go 1.22 routing patterns (`r.Pattern`), which keeps
cardinality bounded to registered routes plus `unmatched`. Metric names are a
contract between the API and the dashboards; renaming one silently empties a
panel or disables an alert.

## Network isolation

`kubernetes/base/networkpolicy.yaml` declares default-deny plus the three flows
the system actually needs: DNS, API to PostgreSQL, and Prometheus to `/metrics`.
Kubelet probes and `kubectl port-forward` bypass the CNI data path and need no
rule.

The policies are enforced by kindnet on the current local cluster and have been
tested for the declared API-to-database and monitoring-to-metrics paths. This is
defence in depth, not a substitute for application authorization.
