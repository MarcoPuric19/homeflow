# Regulatory position

**This is an engineering assessment, not legal advice.** It is written by the
people who build the system, to record which regimes were considered and why
they do or do not apply. Where a threshold decides the answer, the threshold is
named so the reasoning can be checked — and re-checked when the facts change.

Nothing here claims compliance with anything. A claim of compliance is a legal
statement; what follows is a scoping argument.

## The facts everything below depends on

| | |
| --- | --- |
| What it is | A household application for **two private individuals** |
| Where it runs | One privately owned laptop, in a local Kubernetes cluster |
| Reachability | Loopback only. No ingress, no tunnel, no DNS, no public instance |
| Users | Exactly two, administratively created; no public registration |
| Commercial activity | **None.** Not sold, not licensed for a fee, no donations, no paid support, no advertising, no monetisation of any kind |
| What is published | Source code on GitHub, so that it can be read |

Every conclusion below rests on these six lines. If one changes, re-read this
file before assuming the conclusion still holds.

## NIS2 — Directive (EU) 2022/2555

**Does not apply.**

NIS2 binds *entities* operating in the sectors of its Annexes I and II — energy,
transport, banking, health, digital infrastructure, ICT service management,
public administration and others — and, as a rule, only those meeting the
size-cap thresholds of a medium-sized enterprise (broadly: 50 or more staff, or
turnover and balance sheet above EUR 10 million). Member State law transposes
it; in Germany that is the NIS2 implementation act.

Two independent reasons put HomeFlow outside it:

1. **There is no entity.** Two private individuals running software for
   themselves are not an operator of any kind. NIS2 does not regulate private
   persons.
2. **There is no sector and no service.** HomeFlow provides no service to third
   parties. Nothing is offered to anyone, so no sector classification arises.

The size thresholds are therefore never reached in the first place.

**What would change the answer:** operating HomeFlow — or anything derived from
it — as a service for third parties inside one of the Annex sectors, at
sufficient size. Publishing the source code does not do this. Making software
available is not operating a service.

## Cyber Resilience Act — Regulation (EU) 2024/2847

**Does not apply — and this is the regime worth understanding properly,**
because unlike NIS2 it is genuinely about *publishing software*.

The CRA binds manufacturers who make products with digital elements available
**on the market in the course of a commercial activity**. Free and open-source
software supplied outside a commercial activity is excluded; the Regulation
also creates a lighter category of *open-source software steward* for
foundations and similar bodies that support development on a non-profit basis.

HomeFlow is published without any commercial activity: no price, no donations,
no paid support, no monetised distribution. It falls outside the scope.

The dates matter for reading the news correctly rather than for HomeFlow: the
Regulation entered into force in December 2024, the reporting duties of
Article 14 begin to apply in September 2026, and the remaining obligations in
December 2027.

**What would change the answer:** monetising it in any form — selling it,
charging for support, accepting donations tied to the software, or bundling it
into a commercial offering. At that point the manufacturer obligations,
including an SBOM, coordinated vulnerability disclosure and the reporting
duties, become a real programme of work and not a paragraph in a README.

Two things this project already does anyway, because they are good practice
rather than because they are required: a documented vulnerability contact
([`SECURITY.md`](../../SECURITY.md)) and a complete third-party inventory
([`dependencies.md`](dependencies.md)).

## GDPR — Regulation (EU) 2016/679

**Applies to the world, but not to this processing** — Article 2(2)(c) excludes
processing by a natural person in the course of a purely personal or household
activity. Two people keeping their own household and pet records on their own
laptop is the textbook case of that exemption.

Publishing the source code does not change this, **provided no personal data is
published with it.** That is the one real legal risk in this whole file, and it
is an engineering problem, not a scoping argument:

- `.env`, local secret overlays, `storage/receipts/` and `backups/` are ignored
  by Git and must stay ignored.
- The repository's own history was scanned before publication; the export
  script refuses to finish if excluded paths survive the rewrite.
- The commit metadata carries the author's own name and address. That is the
  author's data and his decision.

**What would change the answer:** opening HomeFlow to anyone beyond the two
owners — a third user, a friend's pet data, a shared instance. The household
exemption ends where other people's data begins, and at that point a lawful
basis, information duties and data subject rights all become real.

## Product liability — Directive (EU) 2024/2853

**Does not apply.** The recast directive brings software within the definition
of a product, but liability attaches to products placed on the market in the
course of a commercial activity; free and open-source software supplied outside
such activity is excluded. The same trigger as the CRA applies: monetisation.

The Apache-2.0 warranty disclaimer is not what protects here — a licence term
cannot displace statutory product liability. Scope does.

## Things that are sometimes raised and do not apply here

- **AI Act.** HomeFlow contains no AI system. The planned OCR path uses
  Tesseract, classical optical character recognition, and
  [ADR 0009](../adr/0009-ocr-engine.md) constrains it to extraction only.
- **Medical device regulation.** The application is explicitly barred from
  diagnosing, recommending therapy, calculating dosages or interpreting lab
  values; it records what a veterinarian decided. It is also veterinary, which
  is outside the human medical device regime entirely. The boundary is a
  product rule, not a legal defence — but it keeps the question from arising.
- **Impressum (§ 5 DDG).** The duty attaches to business-like telemedia
  services. A private source code repository is not one, and there is no
  public instance to attach it to.
- **Accessibility (EAA / BFSG).** Binds economic operators offering products
  and services to consumers. No service is offered.

## Summary

| Regime | Applies | Decided by |
| --- | --- | --- |
| NIS2 | No | No entity, no sector, no service to third parties |
| Cyber Resilience Act | No | No commercial activity |
| GDPR | No (household exemption) | Two private individuals, own data, own device |
| Product liability (2024/2853) | No | No commercial activity |
| AI Act, MDR, DDG, BFSG | No | No AI system, veterinary, no business-like service |

**One trigger dominates the table: commercial activity.** Three of the five
entries turn on it alone. The moment HomeFlow is sold, supported for a fee, or
takes donations, this file is obsolete and needs a lawyer rather than an
engineer.

The second trigger is a **third user**. It ends the household exemption, and it
is a single database constraint away — which is exactly why the two-user limit
is enforced in migration `000002` and not merely in the application layer.
