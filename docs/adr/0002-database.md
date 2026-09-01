# ADR 0002: Local PostgreSQL

- Status: Accepted
- Date: 2026-08-31

## Decision

Use containerized PostgreSQL 17 with versioned SQL migrations. Structured data
uses UUID primary keys, UTC timestamps, integer money amounts, constraints and
an event outbox. Receipt bytes are not stored in the database.

## Rationale

PostgreSQL is open source, reliable, supports transactions and is appropriate
for calendar, finance and audit relationships. A single instance is sufficient
for two users.

## Consequences

The database needs a local persistent volume and backups. Since Sprint 2,
embedded migrations and the Shopping repository use PostgreSQL directly. An
integration test refuses to run unless the target database name ends in
`_test`.
