# ADR 0003: Lightweight first-party authentication

- Status: Accepted for local version 1
- Date: 2026-08-31

## Decision

Keep authentication behind an `Authenticator` interface. Store passwords with
Argon2id using per-password random salts and bounded parameters. Use opaque,
random, short-lived access tokens and rotating refresh tokens; persist only
token hashes. Never provide public registration.

Sprint 2 implements Argon2id hashing, authenticated request middleware, exactly
two bootstrap users, login, opaque token persistence, refresh-token rotation,
reuse-family revocation, logout, a local login rate limiter and security audit
events. A future OIDC adapter remains possible.

## Rationale

For two users, running a full identity platform would consume disproportionate
resources. Opaque tokens simplify revocation and avoid placing claims in
client-readable JWTs. Interfaces allow a later OIDC provider.

## Security constraints

- Bootstrap passwords enter only through ignored local environment/Secret data.
- Login errors remain generic and login attempts are limited per IP/username.
- Access tokens expire after 15 minutes and refresh tokens after 30 days.
- Reusing a rotated refresh token revokes the user's entire token family.
- Browser refresh tokens will use `Secure`, `HttpOnly`, `SameSite=Strict`
  cookies and CSRF protection.
