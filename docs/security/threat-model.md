# HomeFlow threat model

## Assets and trust boundaries

Assets include credentials, access/refresh tokens, household schedules,
financial records, receipts, audit events and backups. Trust boundaries exist
at clients, remote-access edge, HTTP API, PostgreSQL, receipt storage,
containers and every optional integration.

| Threat | Primary mitigations | Residual risk / follow-up |
| --- | --- | --- |
| Stolen passwords | Argon2id, unique salt, strong admin-created passwords, optional TOTP later | compromised client and password reuse |
| Brute force | per-account/IP rate limits, generic errors, delay/backoff, audit events | distributed attacks require edge controls |
| Compromised refresh token | hashed storage, rotation, reuse detection, revocation, device/session list | endpoint compromise remains serious |
| Stolen laptop | full-disk encryption recommended, OS lock, encrypted backups, no secrets in Git | physical access while unlocked |
| Publicly reachable API | private VPN preferred, TLS, deny-by-default ingress, auth on every private route | misconfigured router/ingress |
| Broken access control / cross-household access | central principal middleware; Pet domains, calendar, shopping, expenses and receipt access join `household_members`; composite household foreign keys; cross-household integration tests; private calendar ownership checks | logic regressions in future queries |
| Manipulated receipt or medical-document upload | size/type limits, magic-byte validation, SHA-256, generated storage keys, separate financial/medical metadata tables | parser vulnerabilities |
| Path traversal | never use client filenames as paths; opaque server-generated keys and fixed root | storage adapter bugs |
| Malicious PDFs/images | store as untrusted, no inline execution, sandbox future OCR, resource limits | zero-day decoder issues |
| SQL injection | parameterized queries, least-privilege DB role, no SQL string concatenation | unsafe future migrations/queries |
| SSRF | connector URL allowlists, no user-controlled fetch URLs, egress restrictions later | compromised integration config |
| XSS | JSON API, output encoding in future frontend, CSP/security headers | future UI rendering bugs |
| CSRF | bearer header for mobile; SameSite/CSRF token for cookie flows | browser integration mistakes |
| Dependency supply chain | few dependencies, pinned modules/images, checksums, review updates | upstream compromise |
| Container escape | non-root, read-only filesystem, dropped capabilities, seccomp, no token mount | kernel/runtime vulnerabilities |
| Compromised smart-home integration | disabled by default, isolated credentials/scopes, timeouts, audit and revocation | provider account compromise |

## Sprint 3 receipt controls

- request and file size are capped at 10 MiB plus bounded multipart overhead;
- JPEG, PNG and PDF are recognized by magic bytes, not client MIME headers;
- client filenames are reduced to metadata-only basenames;
- storage keys are random UUIDs with server-selected extensions;
- files use mode `0600` and a fixed non-public storage root;
- SHA-256 is calculated while writing;
- failed metadata persistence removes the newly written file;
- receipt downloads require access to the parent expense, force attachment
  disposition, pin the stored MIME type, add a sandbox CSP and never expose the
  storage key; no inline preview, OCR or parser endpoint exists.

Magic-byte validation does not prove that a document is harmless. Future OCR or
preview processing must run sandboxed with strict CPU, memory and time limits.
Medical documents reuse these byte-level controls and additionally require a
household-scoped pet relationship for upload, listing and download. They are
always downloaded as attachments with a pinned MIME type and sandbox CSP.

Pet measurements and reminders use the same household membership predicates as
the other pet domains. Reminder horizons are limited to 366 days and 100 rows;
private pet appointments remain visible only to their owner. Stored laboratory
reference ranges are user-provided documentation and are never interpreted as
medical advice.

## Sprint 4 browser controls

- access and refresh tokens are never exposed to frontend JavaScript;
- authentication cookies are HttpOnly and SameSite=Strict;
- refresh cookies are limited to the browser authentication path;
- state-changing cookie requests require a random CSRF cookie/header pair;
- bearer authentication remains available for future mobile clients and is not
  subject to browser cookie CSRF;
- CSP blocks third-party scripts, styles, connections, frames and form targets;
- no analytics, CDN, external font or runtime API is used.

`Secure=false` is a deliberate local-only compromise for HTTP on loopback. It
is not acceptable for LAN or internet exposure. Remote access remains blocked
until an approved TLS/private-VPN design is implemented and tested.

## Sprint 5 monitoring controls

- metrics carry no credentials, tokens, receipt content, amounts or free-text
  notes; they are counters, histograms and small gauges only;
- Prometheus, Grafana and kube-state-metrics are `ClusterIP` and reachable only
  through an explicit loopback port-forward;
- Grafana analytics, update checks, news and external plugins are disabled;
- the Grafana admin password is generated once into a Secret and never printed
  unless explicitly requested;
- alert rules are evaluated locally and never leave the laptop.

`/metrics` is intentionally unauthenticated. It is registered on the public mux,
so anything that can reach the API port can read it. Inside a loopback-only
cluster this is acceptable and keeps the scrape configuration trivial. It is
**not** acceptable once the API is reachable beyond loopback: route names, request
volumes and the count of due expenses are a usable side channel about household
activity. Before remote access, `/metrics` must be bound to a separate listener,
restricted by network policy, or placed behind authentication. This is added to
the gates below.

## Kubernetes network isolation is declared, not enforced

`kubernetes/base/networkpolicy.yaml` declares default-deny for the `homeflow`
namespace. Under kind's default CNI (kindnet) NetworkPolicy objects are accepted
by the API server and never enforced, so **the cluster currently provides no
network isolation between pods**. Compose does isolate (`internal: true`); the
Kubernetes path does not. Any statement about pod-level segmentation is therefore
about intent, not about an implemented control, until a policy-capable CNI is in
place and enforcement has been verified by test. The threat "publicly reachable
API" above is unaffected — it is mitigated by the absence of an ingress, not by
network policy.

## Security gates before remote access

Remote access is blocked until production authentication with refresh rotation,
rate limiting, authorization tests, TLS/private VPN, secure secret injection,
backup/restore testing and upload validation are complete.
