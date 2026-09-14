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
- Generate a high-entropy idempotency key before RFQ submission. In the RFQ transaction, persist the key, a hash of the versioned canonical submission model, the RFQ ID, and a replayable completion receipt. The same key and payload returns the original result; the same key with a different payload returns 409 and never overwrites the RFQ. Do not hash raw JSON bytes. CSRF tokens, sessions, and idempotency keys are separate mechanisms.
- A no-results search preserves the raw query. It shows a user-initiated “Request this part” action and must not auto-open, auto-create, or auto-submit an RFQ.
- Important domain changes, required Admin Log records, durable work intent, and operation receipts belong in the same transaction when the Architecture requires them. An in-memory queue is only a wake-up mechanism.
- Public search uses a rebuildable, versioned Unicode case-folded projection produced by the application. Indexing and queries use the same fixed algorithm and Unicode version. This does not change trim-and-case-sensitive business identity and does not imply accent/diacritic stripping. Escape `%` and `_` and bind the resulting `LIKE` query parameters.

## Public data and files

- Build every public representation from an explicit `PublicView` allowlist. Never serialize an Admin DTO or database model directly to HTML, JSON-LD, JSON, Markdown, manifests, or `llms.txt`.
- A Product publication unit atomically activates its core HTML, JSON-LD, JSON, Markdown, route, and public revision. Aggregate pages, search projections, Sitemap, and manifests may converge afterward only through a safe projection/fallback that excludes revoked data; otherwise the affected aggregate is temporarily unavailable. Website Configuration and global URL-pattern changes retain a site-wide preflight and atomic switch.
- Only Current + Published data may become newly public. Visibility checks happen before ETag/304 handling. Hide/Archive must revoke every Prods-controlled route, aggregate, machine format, asset reference, and stale validator before reporting success.
- Public revocation is linearized at request admission. After Hide/Archive succeeds, no new request may pass `PublicGate`, including stale-ETag and Range requests. A transfer admitted before revocation may finish; the system does not claim to recall bytes already authorized or delivered.
- Do not expose `data/`, `backups/`, `work/`, `logs/`, configuration, secrets, or generated version directories through a generic filesystem server.
- Treat managed assets as immutable content addressed by stable IDs. Validate streamed content, stage privately, persist the file before its database reference, and publish only through the public access policy.
- Resolve default paths from the executable directory, not the current working directory. Keep the precedence `CLI > environment > one config file > built-in defaults` and report where effective values came from.

## Reliability and security

- Installer, Normal, Maintenance, Upgrade/Restore, and Recovery Required are different modes with different routes and trust roots. Existing damaged data must never fall through to a fresh installer.
- Use opaque server-side sessions for Admin. Enforce authorization server-side, protect state-changing requests against CSRF, validate trusted proxy and Host configuration, and do not replace this with V1 JWT auth for convenience.
- Backups require a consistent SQLite snapshot paired with the exact immutable assets, configuration, and secret versions referenced by that snapshot, plus an independently readable manifest. A directory with a timestamp is not proof of a valid restore point.
- Cross-root Restore uses a durable journal and fixed-order activation. Before journal state `prepared`, the attempt may be abandoned safely. After `prepared`, every restart must roll the same operation forward and record durable completion evidence for each root; automatic rollback is forbidden. Return to Normal only after final cross-root schema/version/manifest/hash verification. Returning to the previous live state is a separate explicit Restore.
- Graceful shutdown must stop admission, drain or classify accepted work, close workers and database handles in order, and release resource locks. Never report an unproven write outcome after interruption.
- Keep liveness and readiness separate. Do not use readiness failure as proof that the process should be killed.

## PoC and verification discipline

- For POC-01, follow `docs/internal/Prods_Codex_POC_01_Prompt.zh-TW.md`. Work in an isolated spike area, do not deploy, push, open a PR, start POC-02, or claim production readiness.
- Treat POC-01 only as the renderer feasibility gate. It verifies the single Go binary, shared `PublicView`, semantic no-JavaScript HTML/RFQ path, React islands, public formats, and no Node.js runtime. It does not prove publication correctness.
- Treat POC-03 as the publication correctness gate. It must cover revision A to B, first publish, update, Hide/Archive, stale ETag, Range requests, search/category/Sitemap residue, shared-asset authorization, crash points around activation, and restart reconciliation. Do not claim the Public Architecture consistency model is established until POC-03 passes.
- Test both no-JavaScript and enhanced paths, successful and failed RFQ persistence, Hidden data rejection, raw semantic HTML, public machine formats, conditional GET behavior, and browser console/bundle separation.
- A cross-compile is not Windows runtime verification. Record the actual OS/architecture, dependency inventory, commands, raw HTML, test output, console errors, bundle sizes, and every unverified item.
- Prefer deterministic unit and integration tests for domain invariants, transaction boundaries, route visibility, idempotency, and failure recovery. Use browser tests only where browser behavior is the risk being tested.
- Never convert an unverified proposal, benchmark target, suggested default, or successful happy-path demo into a product guarantee.

## Change discipline

- Preserve unrelated and pre-existing user changes. Stage and commit only files owned by the current task unless the user explicitly expands the commit scope.
- Do not edit the PRD or Architecture merely to make an implementation pass. Surface the mismatch with file and line references and ask for a product or architecture decision.
- Keep generated HTML counterparts synchronized when their Markdown source is intentionally changed.
