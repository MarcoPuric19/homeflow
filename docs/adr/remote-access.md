# ADR: Remote access options

- Status: Proposed; no option activated
- Date: 2026-08-31

HomeFlow must not be exposed publicly until authentication, TLS, backups and
operational controls are complete. The provider facts below were checked on
2026-08-31 against official sources. Pricing and terms can change and must be
rechecked immediately before adoption.

| Option | Account/card | CGNAT | Public URL/webhooks | Privacy and limits | Assessment |
| --- | --- | --- | --- | --- | --- |
| Self-hosted WireGuard | none | needs reachable IPv4/IPv6 or router support | no | best data control; manual keys/routing | Preferred when network permits |
| Tailscale Personal | account required; Personal has no billing email; no card needed for the free plan according to official plan material | yes | private by default; public features separate | hosted coordination sees metadata; up to 6 users, unlimited user devices, 50 tagged resources and 1,000 ephemeral-resource minutes/month | Strong convenience candidate, not approved |
| Headscale + WireGuard clients | no vendor account | only if coordination endpoint is reachable | no | self-hosted but more operations; does not itself solve CGNAT | Useful with a reachable home endpoint |
| Native IPv6 + WireGuard/reverse proxy | none | often bypasses IPv4 CGNAT | possible | requires stable inbound IPv6 and strict firewall/router work | Technically clean where ISP supports it |
| Dynamic DNS + Let's Encrypt | provider account may be needed; free choices exist | no for IPv4 CGNAT | yes | exposes a public endpoint; DNS-provider terms vary | Only with public IP and explicit approval |
| Hosted reverse tunnel | usually account; free tiers vary | yes | yes | third party handles traffic/metadata; limits and terms can change | Not selected under the cost/privacy rule |

## Decision direction

Prefer private VPN access over a public URL. First test whether the home
connection offers usable inbound IPv6 or non-CGNAT IPv4. If it does, self-hosted
WireGuard is the leading option. Otherwise, re-evaluate a permanently free
overlay network with current official terms. Webhooks from public providers are
incompatible with a purely private VPN unless a separately approved ingress
mechanism exists.

No account, tunnel, DNS record, certificate, port-forward or firewall rule is
created in Sprint 1 or Sprint 2.

## Official sources checked

- [Tailscale pricing](https://tailscale.com/pricing): Personal is listed as
  `$0`, “free forever”, up to six users; the page documents resource limits but
  no traffic-volume cap. It also warns that custom-domain signups may enter a
  business trial, so a personal account would have to be selected deliberately.
- [Tailscale free-plan documentation](https://tailscale.com/docs/account/manage-plans/free-plans-discounts): six users on Personal.
- [Tailscale contact preferences](https://tailscale.com/kb/1224/contact-preferences): no billing email exists when no paid plan is used.
- [WireGuard](https://www.wireguard.com/): open-source VPN software; no vendor
  account is inherent to self-hosting.
- [Headscale](https://headscale.net/): open-source, self-hosted Tailscale control
  server aimed at personal/small deployments; it still needs a reachable
  coordination endpoint.
- [Let's Encrypt subscriber agreement](https://letsencrypt.org/documents/LE-SA-v1.7-June-04-2026.pdf): certificates are issued free of charge; public DNS
  validation/reachability remains an operational prerequisite.
