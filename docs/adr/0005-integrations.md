# ADR 0005: Ports and adapters for integrations

- Status: Accepted
- Date: 2026-08-31

## Decision

Core modules publish internal events and call provider-neutral connector ports.
Alexa, Ring, Home Assistant, MQTT and webhook adapters are optional and disabled
by default. Sprint 1 includes interfaces and deterministic mocks only.

## Rationale

HomeFlow must work offline and must not inherit a provider's availability,
pricing or data model. Provider credentials stay outside Git and each connector
gets explicit network allowlists, timeouts and least-privilege scopes.

## Provider research (checked 2026-08-31)

### Alexa

The [official custom-skill documentation](https://developer.amazon.com/en-US/docs/alexa/custom-skills/host-a-custom-skill-as-a-web-service.html)
requires a self-hosted skill endpoint to be internet-accessible on port 443 with
TLS and verified Alexa requests. Amazon documents the
[developer account as free](https://developer.amazon.com/en-US/docs/alexa/custom-skills/steps-to-build-a-custom-skill.html),
but its default Lambda route is an AWS service/free-tier path and is rejected by
HomeFlow's permanent-zero-cost rule. A home-hosted endpoint might avoid software
fees, but it requires public ingress, trusted certificates and possibly account
linking/privacy-policy work. Therefore no Alexa adapter beyond the mock is
approved.

### Ring

Ring now documents an [official developer platform](https://developer.ring.com/)
using authorization-code bearer tokens, webhooks, developer registration and
app certification. However, Ring states that compatible subscriptions may be
required, and its [Appstore support documentation](https://ring.com/support/articles/e2sg9/Ring-appstore-third-party-integrations)
says a Ring subscription is required to use developer apps. This conflicts with
the unconditional zero-additional-cost requirement. No Ring API, credentials or
unofficial reverse-engineered client is implemented.

### Local adapters

Home Assistant and MQTT remain promising local adapters but are not installed
in Sprint 1. Webhooks require strict signature verification, allowlists,
timeouts and SSRF controls before activation.
