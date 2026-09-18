# Prods engineering case study

English | [繁體中文](../zh-TW/engineering-case-study.md) | [简体中文](../zh-CN/engineering-case-study.md)

[README](../../README.md) · [Architecture](architecture.md) · [Operations and evidence limits](operations.md)

## Context and evidence

A technical B2B catalog has to preserve distinctions that a name/description/image product list cannot express. A semiconductor's voltage range, a connector's contact count and a module's interface are different specifications. Raw values may include conditions or tolerances. Manufacturer identity, part number, lifecycle and datasheets help a buyer identify a part, but none establishes current stock or suitability as a replacement.

Prods connects administrator-maintained catalog data to public discovery and RFQ intake. The same inquiry workflow accepts catalog products and visitor-entered Requested Parts, so an incomplete catalog does not require inventing product records. The implementation has category/specification models, public publication protocols, an Admin application and installation/recovery paths. It does not establish customer adoption or a running production deployment.

This account was checked against the working tree on 2026-09-17 (`VERSION`: `v0.6.8`). Links below point to public source and tests. Internal requirements and historical ADRs remain local; this document neither republishes them nor creates retrospective ADRs. Alternatives below are architectural comparisons, not claims that every alternative was prototyped. Evolution triggers are review criteria proposed by this guide.

## Requirements that drove the implementation

| Requirement | Observable implementation consequence |
| --- | --- |
| A small team can operate a self-hosted catalog | One executable embeds the UI and runs SQLite-backed application services. |
| Buyers and crawlers can read core content without JavaScript | Go produces usable HTML; React enhances selected controls. |
| Saved data and public visibility must be distinguishable | Publication intents and active revisions separate edits from public activation. |
| Repeated RFQ submissions must have an inspectable outcome | Canonical payload hashes and durable receipts share the RFQ transaction. |
| Technical data differs by category and can be incomplete | Spec Sets, raw values and derived normalization records are separate. |
| Damaged state must not be mistaken for a new site | Startup inspection, Installer and journaled Recovery have explicit boundaries. |

Traffic volume, revenue, customer count, high availability and a need for distributed services are not established requirements or measured results in this account.

## 1. One process with explicit modules

**Decision.** Run HTTP, application services and bounded background work in one Go executable.

**Context.** A self-hosted catalog already needs persistent data, asset files and a recovery procedure. Requiring separate API, renderer, queue and search deployments would add operational dependencies to those responsibilities.

**Alternatives considered.** Separate services and an external queue are a useful comparison when components require independent scaling or failure isolation; no such production requirement is established here.

**Why this choice.** [`main.go`](../../cmd/prods/main.go) wires the services, while [`publishing.Repository`](../../internal/publishing/protocol.go) and the [`backup Snapshotter`](../../internal/recovery/backup.go) make persistence dependencies explicit. SQL-backed work records retain intent across restarts without a queue server.

**Trade-offs.** Packaging and local diagnosis are simpler. CPU, memory, process failure and maintenance remain shared. Package boundaries make reasoning and testing easier; they do not make a later service split free, particularly around transactions and visibility.

**Failure / evolution trigger.** Representative measurements show that workers cannot coexist with interactive traffic within resource limits, or a deployment needs independently available components. Establish that evidence before introducing network boundaries.

## 2. SQLite with controlled writes

**Decision.** Use SQLite as the primary database and SQL-first repositories with serialized write admission.

**Context.** Catalog reads, short Admin/RFQ writes and occasional large imports have different costs. Atomic import and durable inquiry capture need a clear contention contract.

**Alternatives considered.** A database server can support different concurrency and operational arrangements; it also adds provisioning, access and backup responsibilities. Splitting an import into partial commits would shorten individual transactions but change the all-or-nothing contract.

**Why this choice.** [`beginWrite`](../../internal/storage/sqlite/store.go) provides one in-process write admission point. [`import_store.go`](../../internal/storage/sqlite/import_store.go) validates and rechecks the import at commit. Backups pair a consistent DB snapshot with referenced assets and other required roots.

**Trade-offs.** There is no database server to operate, but writes contend. A long final import transaction may delay RFQs. The writer channel is not a priority/fairness guarantee. Backing up a DB file alone is insufficient when records refer to external files and secrets.

**Failure / evolution trigger.** Measure queue wait, RFQ latency, import transaction duration and restore time against an operator-agreed target. Revisit preparation, scheduling or storage if that workload cannot meet the target. This guide supplies no invented throughput ceiling or SLA.

**Evidence to inspect.** [`Atomic import tests`](../../internal/storage/sqlite/import_store_test.go) cover revision rechecks, receipt replay and rollback; [`backup tests`](../../internal/recovery/backup_files_test.go) cover paired files. The tagged [`POC-02 harness`](../../internal/storage/sqlite/poc02_test.go) is an experiment, not production capacity evidence.

## 3. Separate Public rendering from Admin interaction

**Decision.** Generate Public HTML in Go and use isolated React enhancements; use React/Refine/Ant Design for the Admin application.

**Context.** Product information, search links and basic RFQ forms need to work for visitors and crawlers before JavaScript runs. Administrators need interactive lists, forms, previews and job status.

**Alternatives considered.** A single full-page SPA would put core Public behavior behind client execution. A separate Node rendering service would change the runtime distribution model. Server-only Admin forms would avoid client state but give up the chosen management framework.

**Why this choice.** [`Public templates`](../../internal/webapp/templates/pages.tmpl) and [`publication rendering`](../../internal/publishing/render.go) produce the content. [`Public roots`](../../web/ui/src/public/main.tsx) add RFQ selection/listing controls. Separate Vite entry points embed Admin and system applications without turning Public into an Admin bundle.

**Trade-offs.** Two presentation models require consistent localization and behavior. Shared published data reduces drift across HTML/JSON/Markdown, but does not automatically prove every rendering path correct. Go-generated HTML is not React SSR.

**Failure / evolution trigger.** New interactions cannot be implemented as bounded enhancements without duplicating core data behavior, or measured rendering/maintenance costs justify another model while retaining the no-JavaScript core journey.

**Evidence to inspect.** [`Search/listing HTML tests`](../../internal/webapp/search_test.go), [`machine-format tests`](../../internal/publishing/machine_test.go) and [`localized-unit tests`](../../internal/publishing/localized_unit_test.go).

## 4. Domain commands retain their own outcomes

**Decision.** Use the Refine DataProvider for supported resources and named adapters for meaningful transitions. Keep publication and RFQ success tied to their own durable evidence.

**Context.** Editing a field, archiving a product, publishing a website and retrying an inquiry do not have interchangeable effects. A lost response does not prove a failed write.

**Alternatives considered.** Mapping everything to generic CRUD, optimistic UI updates or automatic mutation retries would simplify some client code but obscure revisions, partial success and ambiguous outcomes.

**Why this choice.** [`dataProvider.ts`](../../web/ui/src/admin/dataProvider.ts) rejects Product delete; [`catalogCommands.ts`](../../web/ui/src/admin/catalogCommands.ts) names Archive, Hide, Clone and preview. Publish currently uses a revisioned Product PUT: the boundary is semantic, not a promise of one endpoint per command. [`api.ts`](../../web/ui/src/admin/api.ts) tells users to inspect saved state when a write's outcome is unknown. [`AdminProviders.tsx`](../../web/ui/src/admin/AdminProviders.tsx) disables mutation retry and optimistic mutation mode.

The same discipline reaches persistence. [`Publication`](../../internal/publishing/protocol.go) stages a unit before activating it, and revocation denies new admission before ETag/Range processing. [`SubmitRFQ`](../../internal/storage/sqlite/store.go) stores the canonical payload hash and receipt in the RFQ transaction. Same-key/same-payload replay differs from a conflicting edit. Product Bulk permits per-product partial success; Excel import is whole-batch atomic.

**Trade-offs.** Adapters and explicit outcome handling require more code. A publication save may precede public activation; users need state they can inspect. SMTP can have an uncertain result and cannot be treated as exactly-once delivery.

**Failure / evolution trigger.** Commands bypass the shared transport, receipts cannot reconcile interruptions, or tests show stale data becoming public. Fix the contract and its evidence before hiding the problem with automatic retries.

**Evidence to inspect.** [`Adapter tests`](../../web/ui/src/admin/adapters.test.ts), [`normal publication tests`](../../internal/webapp/publication_test.go), [`POC-03 protocol tests`](../../internal/publishing/poc03_test.go), [`RFQ receipt tests`](../../internal/storage/sqlite/rfq_receipt_test.go). Renderer feasibility and publication correctness are separate gates; neither is proof of production readiness.

## 5. Category-specific data with controlled presentation

**Decision.** Model specifications separately from Products, reuse Spec Sets, retain raw values and track normalization as derived data. Configure category listing profiles separately from those values.

**Context.** A giant flat product schema would mix unrelated component properties. Treating every raw string as a reliable scalar would misrepresent units, ranges, conditions and missing data.

**Alternatives considered.** One column per possible property creates schema churn; a free-form blob alone gives little support for applicability, provenance or controlled filtering. An arbitrary per-product page layout would combine data maintenance with presentation design.

**Why this choice.** [`maintenance.go`](../../internal/catalog/maintenance.go) defines SpecValue and NormalizedValue separately, including source revision, semantic version and status. [`spec_document_store.go`](../../internal/storage/sqlite/spec_document_store.go) reconciles applicability. [`listing.go`](../../internal/cataloglisting/listing.go) derives Common columns from the full scope's applicability rather than the first row or current page. Product identity remains independent of paths and search folding.

**Trade-offs.** Applicability, stale normalization and category changes require explicit reconciliation. The model and normalization records do not establish a general electrical parser or prove two parts compatible. Structured configuration is less flexible than arbitrary templates, but preserves a reviewable publishing boundary.

**Failure / evolution trigger.** Real categories cannot express required properties or users repeatedly need unsupported calculations. Add a defined data/normalization contract with examples and failure behavior before expanding layout or inference capabilities.

**Evidence to inspect.** [`Common/profile tests`](../../internal/cataloglisting/listing_test.go), [`catalog maintenance tests`](../../internal/catalog/maintenance_test.go), [`blank-manufacturer identity tests`](../../internal/storage/sqlite/d9_identity_test.go).

## 6. Installation and recovery are lifecycle boundaries

**Decision.** Distinguish fresh, installing, ready, upgrade-required and recovery-required state. Treat restore as a journaled operation with roll-forward after preparation.

**Context.** A missing initial database and a damaged existing database require opposite responses. Restoring DB, assets and secrets across roots cannot be represented by one filesystem rename.

**Alternatives considered.** Automatically initializing on any open error risks replacing evidence of an existing site. Copying roots without a journal cannot distinguish a completed step from an interrupted one. Returning to an older point is a separate restore, not an implicit undo after preparation.

**Why this choice.** [`Inspect` and `CompleteInstallation`](../../internal/storage/sqlite/store.go) classify state and commit the initial Owner/Ready state together. [`recovery_ui.go`](../../cmd/prods/recovery_ui.go) provides a separate repair entry. [`restore.go`](../../internal/recovery/restore.go) records per-root hashes and progress, resumes the same operation and verifies all roots before completion.

**Trade-offs.** Startup and recovery have more states and more code. A prepared operation may require repair before normal service resumes. Local manifests and read-only files do not provide off-host disaster recovery, tamper-proof storage or a recovery-time guarantee.

**Failure / evolution trigger.** An interrupted step cannot be reconciled from durable evidence, platform file operations behave differently, or the operator's recovery target exceeds measured restore behavior. Test the actual platform/storage topology and revise the protocol deliberately.

**Evidence to inspect.** [`Installation tests`](../../internal/storage/sqlite/installation_test.go), [`restore fault-boundary tests`](../../internal/recovery/restore_fault_test.go), [`shutdown tests`](../../cmd/prods/shutdown_test.go). Synthetic failures and cross-compilation do not replace native power-loss, filesystem or service-manager acceptance.

## Scope and AI positioning

**Unpublished local work, excluded from this documentation commit:** the reviewed working tree contained `internal/webapp/brand_import.go` and `web/ui/src/admin/brandImportLabels.ts`, implementing a human-mediated external ChatGPT workflow. Prods generates a request-bound prompt/schema; the operator runs it outside Prods and pastes JSON back. Validation checks request/version/source-URL bindings and produces a proposed diff. Explicit approval with an expected revision saves Website Working Copy; publication remains separate. Structural validation does not independently verify the external model's observations. This code was already in progress during this review; no release or end-to-end external-model acceptance is claimed. It is not a built-in inference or RAG service.

- **Implemented:** catalog maintenance, category/specification data, public discovery, publication, RFQ intake, Admin and lifecycle/operations code described above; JSON/Markdown/`llms.txt` are readable outputs.
- **Planned or deferred:** broader content composition/headless capabilities are outside current V1 delivery; this guide makes no commitment to dates or implementation designs.
- **Exploratory:** AI-assisted RFQ extraction could be evaluated only against a real workflow, with explicit confidence, verification and failure boundaries. It is not documented as an implemented feature.
- **Out of scope:** checkout, pricing/inventory promises, CRM, arbitrary per-entity layouts or JavaScript, external write tokens, authoritative cross-manufacturer replacements, an AI chatbot or RAG service. Semantic similarity is not evidence of electrical compatibility.

The repository supports a concrete account of product boundaries, implementation and operational mechanisms. Establishing a running production system additionally requires deployment-specific evidence; [operations](operations.md) lists what is missing.
