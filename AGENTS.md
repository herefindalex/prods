# Prods engineering instructions

## Source of truth

- Read `docs/internal/Prods_PRD_v0.5.zh-TW.md` and `docs/internal/Prods_Architecture_v0.1.zh-TW.md` before designing or implementing Prods.
- Product requirements and confirmed decisions take precedence over architectural proposals. In the Architecture document, `P` and `E` are requirements/confirmed constraints; `A` entries are proposals that still require review or PoC evidence; `T` entries are supporting technical references.
- If the PRD, Architecture, and a task prompt appear inconsistent, report the exact conflict and stop before silently choosing one interpretation.
- Do not change a requirement merely because a framework, library, ORM, router, builder, or driver prefers a different design.
- This is a new implementation. Do not reuse an old Prods codebase or assume compatibility with an old API, schema, or UI unless a later requirement explicitly says so.

## Fixed V1 boundaries

- Ship one Go process and one platform-specific binary for Windows and Linux. Build tooling may use Node.js, but the user runtime must not require Node.js, a database server, Docker, Hugo, an external renderer, Redis, RabbitMQ, or an external search service.
- Use SQLite as the long-term primary database with SQL-first, thin repositories and binary-managed migrations.
- Admin uses React with Ant Design v6 and ships as embedded build output. Public pages do not use Ant Design.
- Public entity pages use Go templates/partials to produce complete semantic HTML. React may enhance isolated interaction roots; it must not own or erase core product content, search, pagination, document links, or the basic RFQ path.
- Core public flows must remain usable without JavaScript. Do not describe Go-rendered HTML as React SSR or hydrate arbitrary Go-owned DOM.
- Keep V1 a modular monolith. Do not introduce microservices, an external queue, a generic workflow engine, a page-builder engine, external write tokens, competitive cross-reference, commerce, CRM, arbitrary custom JavaScript, or other explicitly deferred scope.

## Domain and persistence invariants

- Keep stable opaque domain IDs independent of SQLite row IDs, URLs, display names, and filesystem paths.
- Do not add a database unique or partial-unique constraint for Manufacturer Part Number. Every controlled Product write path must recheck Current identity and expected revision inside the admitted write transaction.
- SQLite has one admitted writer. Interactive writes stay short and synchronous. Large imports prepare outside the final transaction, then commit atomically through the same admission path. Do not claim atomic import and zero RFQ wait without measured evidence.
- A successful RFQ response means the RFQ, all items, snapshots, recipients, and idempotency receipt have committed. Do not auto-send SMTP, require price/availability, or create a Product from a requested/uncatalogued part.
- A no-results search preserves the raw query. It shows a user-initiated “Request this part” action and must not auto-open, auto-create, or auto-submit an RFQ.
- Important domain changes, required Admin Log records, durable work intent, and operation receipts belong in the same transaction when the Architecture requires them. An in-memory queue is only a wake-up mechanism.

## Public data and files

- Build every public representation from an explicit `PublicView` allowlist. Never serialize an Admin DTO or database model directly to HTML, JSON-LD, JSON, Markdown, manifests, or `llms.txt`.
- Only Current + Published data may become newly public. Visibility checks happen before ETag/304 handling. Hide/Archive must revoke every Prods-controlled route, aggregate, machine format, asset reference, and stale validator before reporting success.
- Do not expose `data/`, `backups/`, `work/`, `logs/`, configuration, secrets, or generated version directories through a generic filesystem server.
- Treat managed assets as immutable content addressed by stable IDs. Validate streamed content, stage privately, persist the file before its database reference, and publish only through the public access policy.
- Resolve default paths from the executable directory, not the current working directory. Keep the precedence `CLI > environment > one config file > built-in defaults` and report where effective values came from.

## Reliability and security

- Installer, Normal, Maintenance, Upgrade/Restore, and Recovery Required are different modes with different routes and trust roots. Existing damaged data must never fall through to a fresh installer.
- Use opaque server-side sessions for Admin. Enforce authorization server-side, protect state-changing requests against CSRF, validate trusted proxy and Host configuration, and do not replace this with V1 JWT auth for convenience.
- Backups require a consistent SQLite snapshot paired with the exact immutable assets, configuration, and secret versions referenced by that snapshot, plus an independently readable manifest. A directory with a timestamp is not proof of a valid restore point.
- Graceful shutdown must stop admission, drain or classify accepted work, close workers and database handles in order, and release resource locks. Never report an unproven write outcome after interruption.
- Keep liveness and readiness separate. Do not use readiness failure as proof that the process should be killed.

## PoC and verification discipline

- For POC-01, follow `docs/internal/Prods_Codex_POC_01_Prompt.zh-TW.md`. Work in an isolated spike area, do not deploy, push, open a PR, start POC-02, or claim production readiness.
- Test both no-JavaScript and enhanced paths, successful and failed RFQ persistence, Hidden data rejection, raw semantic HTML, public machine formats, conditional GET behavior, and browser console/bundle separation.
- A cross-compile is not Windows runtime verification. Record the actual OS/architecture, dependency inventory, commands, raw HTML, test output, console errors, bundle sizes, and every unverified item.
- Prefer deterministic unit and integration tests for domain invariants, transaction boundaries, route visibility, idempotency, and failure recovery. Use browser tests only where browser behavior is the risk being tested.
- Never convert an unverified proposal, benchmark target, suggested default, or successful happy-path demo into a product guarantee.

## Change discipline

- Preserve unrelated and pre-existing user changes. Stage and commit only files owned by the current task unless the user explicitly expands the commit scope.
- Do not edit the PRD or Architecture merely to make an implementation pass. Surface the mismatch with file and line references and ask for a product or architecture decision.
- Keep generated HTML counterparts synchronized when their Markdown source is intentionally changed.
