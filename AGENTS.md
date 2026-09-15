# Prods engineering instructions

## Source of truth and repository boundary

- Read `docs/internal/Prods_PRD_v0.6.zh-TW.md` and `docs/internal/Prods_Architecture_v0.1.zh-TW.md` before design or implementation. This file is a concise guardrail summary, not the authoritative specification.
- Product requirements and approved decisions take precedence over proposals. Architecture `P` inherits the cited PRD's confirmed/suggested/pending status; `E` and `D` are approved constraints; unapproved `A` remains a proposal. See Architecture §0.1 and §2.1.
- If documents conflict, report the exact sections before choosing an interpretation. Do not change requirements to suit a framework, ORM, router, builder, or driver.
- Internal documents and their rendered equivalents stay local and outside source history: never stage, force-add, commit, copy their contents into tracked deliverables, or change ignore rules to include them. Report any unexpectedly tracked internal document; do not rewrite history.
- Preserve unrelated changes. Intentional internal Markdown edits must keep existing HTML counterparts synchronized. Do not edit specifications merely to make an implementation pass.
- This is a new implementation; do not reuse an old Prods codebase or assume old API/schema/UI compatibility without explicit requirements.

## Fixed implementation boundaries

- One Go process and platform-specific binary for Windows/Linux; SQLite is the long-term primary DB with SQL-first thin repositories and binary-managed migrations. Node.js is build-only, not a user runtime dependency. No required database server, Docker, Hugo, external renderer, queue, or search service. See Architecture §2–3 and §7.
- Admin is React + Ant Design v6 with embedded build output; Public does not load Ant Design. Go produces complete semantic HTML. React may enhance isolated roots, never require whole-page hydration or remove the only usable core content/search/pagination/document/basic RFQ path. See §9.
- Keep V1 a modular monolith and preserve the PRD's explicit deferred scope; do not add commerce, CRM, external write tokens, arbitrary custom JS, or a page-builder/workflow engine. See PRD §2.3 and Architecture §21.
- Stable opaque domain IDs are independent of row IDs, URLs and paths. No DB unique/partial-unique constraint for Manufacturer + Part Number; controlled writes recheck Current identity and expected revision inside the admitted transaction. All writes share bounded writer admission; Atomic Import stays all-or-nothing and cannot promise zero RFQ wait. See §6–8.
- Public output uses an explicit `PublicView` allowlist, never an Admin DTO/DB model. No-results preserves raw query and requires a user-initiated Requested Part action; RFQ does not create Products, auto-send SMTP, or require quantity/price/availability. See §9.1 and §12.

## Approved D1–D10: summary and authoritative sections

- **D1 — Product publication unit:** activate core representations, route and public revision together. Aggregates may converge only with safe visibility/links or temporary unavailability; Website/global URL Pattern/prefix changes remain site-wide atomic. Source: Architecture §10.2, §10.4–10.5.
- **D2 — Revocation admission:** after Hide/Archive succeeds, deny new access authorized by the revoked Product, before ETag/Range handling; previously admitted transfers may finish. Preserve other valid shared-asset references. Source: §10.3–10.4 and §13.3.
- **D3 — Separate gates:** POC-01 proves renderer feasibility only; POC-03 owns publication correctness. Never claim the public consistency model proven before POC-03 also passes. Source: §9.4 and §23 POC-01/POC-03.
- **D4 — RFQ receipt:** high-entropy key plus canonical payload hash, RFQ ID and replayable receipt commit in one transaction. Same key/same payload replays; different payload returns 409 and preserves edits. Do not hash raw JSON; keep CSRF, session and idempotency separate. Source: §7.4 and §12.3.
- **D5 — Restore:** durable journal; before `prepared` safe abandonment, after it only same-operation roll-forward with per-root evidence. Complete cross-root verification before Normal; returning to an old point is a later explicit Restore. Source: §4.5 and §15.2.
- **D6 — Search:** application-generated, rebuildable, versioned Unicode-folded projection; query uses the same pinned rule. Preserve case-sensitive identity, escape literal LIKE wildcards, do not strip diacritics or add SQLite ICU. Source: §11.3.
- **D7 — Source-locale default:** a create/import operation defaults source locale from the then-current Site Default, may override it once for the operation, and may correct individual fields through advanced controls. Never infer customer-content provenance from Admin UI locale. Source: Architecture §2.2 and PRD §15.3–15.4.
- **D8 — Initial ten-language resources:** complete Codex-produced defaults may ship after key/placeholder/plural/format/layout checks, but release evidence must say they have not received professional native-language, legal, or marketing review. Keep official definitions/defaults versioned and customer overrides separate. Source: Architecture §2.2 and PRD §15.1, §15.15.
- **D9 — Blank manufacturer identity:** blank Manufacturer and a named Manufacturer are distinct identity domains and may each have a Current Product with the same Part Number. Assigning a named Manufacturer later must recheck the target Current identity and expected revision in the admitted transaction. Source: Architecture §2.2, §6 and PRD §4.1.
- **D10 — Import empty/duplicate contract:** missing columns and blank cells preserve existing values; only explicit `CLEAR` clears optional values. Multiple rows resolving to one Current Product are a whole-import validation error, never first/last-row-wins. Source: Architecture §2.2 and PRD §11.5.

## PRD v0.6 multilingual and bulk guardrails

- V1 has exactly ten built-in locales: `en-US`, `zh-TW`, `zh-CN`, `ja-JP`, `ko-KR`, `de-DE`, `fr-FR`, `it-IT`, `es-ES`, `pt-BR`. Initial Site Default is `en-US`; each Admin user's UI locale is independent of public locale enablement.
- Closing multilingual content editing only hides editing controls. It must not unpublish translations. Locale enable/disable, Site Default, Public Copy, Navigation, Theme, Organization and related SEO belong to Website working/preview/publish; disabled locales retain data but never participate in public resolution after activation.
- Customer Content resolves each translatable value through Requested → Site Default → that value's Source Locale → field fallback/absence, skipping every disabled candidate. Public Copy uses its separate override/default chain. Public representations report actual provenance and share one published `PublicView`/model.
- Public Copy has versioned official definitions/defaults and Website-revision customer overrides. Stable semantic keys define editability, typed allowed/required placeholders, plural/select branches and samples. Reset removes one working `key + locale` override and never publishes immediately.
- Product Bulk supports Publish, Hide, Archive, Change Category and Change Lifecycle with per-product atomic transactions and batch partial success. Retry only unresolved items after a fresh preflight and confirmation. Product Excel Import remains whole-batch atomic.
- V1 keeps Domain Truth, Public Copy, future Customer Content Blocks and Layout separate; it stays same-origin and does not implement a page builder, generic CMS pages, configurable CORS, external write tokens, arbitrary templates/JS, or Enterprise per-item layout overrides. Source: PRD §28 and Architecture §2.2.

## Reliability, files and security

- Important changes, required Audit, durable work intent and receipts share the transaction where specified; in-memory queues are wake-ups only. Never claim a write outcome without durable evidence. See §7.4, §8.4, §12.3 and §20.
- Assets are immutable files referenced by stable IDs, not a content-deduplication scheme. Validate/stage privately and durably place files before DB references. Public authorization follows valid references; never serve private roots or historical generated directories through a generic filesystem server. See §5 and §13.
- Resolve default paths from the executable directory; preserve `CLI > ENV > one config > defaults`. Separate Installer, Normal, Maintenance and Recovery; damaged data never starts a fresh installer. Backups pair consistent DB snapshots with exact assets/config/secrets and independent manifests. See §4–5 and §14–15.
- Use opaque server-side Admin sessions, server-side authorization, CSRF protection and trusted Host/proxy settings. No JWT substitution for convenience. Keep liveness/readiness separate; shutdown drains accepted work before closing DB/locks. See §17–20.

## PoC and evidence discipline

- POC-01 follows `docs/internal/Prods_Codex_POC_01_Prompt.zh-TW.md`; POC-02–06 scope and gate meanings are authoritative in Architecture §23. Root `POC*_REPORT.zh-TW.md` files contain reproducible evidence and limits, but do not replace the PRD/Architecture.
- Current 2026-09-14 result: POC-01 accepted; POC-03 passed within the Linux protocol harness, so the core Public consistency model is feasible in PoC scope. POC-02, POC-04, POC-05 and POC-06 are Partial. None of these results means production readiness or silently approves provisional dependencies.
- Publication smoke tests do not satisfy POC-03. Preserve the D3 renderer/publication gate distinction in future changes and regression evidence.
- Windows cross-compilation is not Windows runtime verification. Record actual platforms, dependencies, commands, raw evidence and every unverified item. Never turn a proposal, suggested default, benchmark target or happy-path demo into a guarantee.
- Do not deploy, push, open a PR, or start full product development automatically after a PoC.

## Current M1 implementation guardrails

- Startup must preserve the `fresh → installing → ready` distinction. Only explicit Installer mode may create a missing database; `installing` resumes with a new claim token/session; unrecognized or damaged data is Recovery Required and must not be initialized over. Source: Architecture §4 and PRD §16; local-only implementation parameters: `docs/internal/M1_BOOTSTRAP_AUTH.md`.
- Normal sites authenticate the first Owner with a versioned password verifier and a durable opaque server-side session whose bearer token is never stored. Temporary token login is restricted to databases durably marked as PoC instances. Source: Architecture §18 and PRD §17; technical parameters are documented separately and remain tunable.
- Installer success must atomically create the first Owner, minimum site settings, system categories, required Admin Log, and Ready marker, then require restart. Do not claim the broader Installer, Recovery, user-lifecycle, or production-security scope complete from this vertical slice. Source: Architecture §4, §18, §25.1.
