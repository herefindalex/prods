# Prods

English | [繁體中文](README-zh_TW.md) | [简体中文](README-zh_CN.md)

**Turn product data into an official, searchable catalog and RFQ website without operating an application stack.**

Prods is a self-hosted product catalog and request-for-quotation system for manufacturers, distributors, and other B2B product teams. It helps a small team import and maintain technical product data, publish it safely across human- and machine-readable formats, and capture real buyer requests from one portable Go binary.

The runtime includes the Admin application, public website, SQLite support, search, publishing, RFQ intake, backup, recovery, and operational tooling. Node.js is only needed when building from source. A deployed site does not require Node.js, a separate database server, Docker, a queue, or an external search service.

## Why Prods

- **Launch with the data you already have.** Manage Products, categories, specifications, dictionaries, documents, translations, and Website settings. XLSX import/export and bulk actions support catalog maintenance without forcing every change through individual forms.
- **Publish with controlled visibility.** Product pages, structured data, JSON, Markdown, and route state move together. Updating a Published Product keeps the previous valid revision online until the replacement is ready; Hide and Archive stop new public access immediately.
- **Serve engineers, buyers, crawlers, and AI tools.** Core browsing, search, pagination, documents, and RFQ work from server-rendered semantic HTML without JavaScript. The same published model feeds JSON-LD, JSON, Markdown, Sitemap, manifest, and `llms.txt` outputs.
- **Capture demand without pretending to be a commerce platform.** Visitors can request one or more catalog Products or submit a Requested Part when search has no result. Prods records durable, replay-safe RFQs and leaves pricing, availability, qualification, and follow-up to the business.
- **Keep operations understandable.** One process owns migrations, bounded writes, durable jobs, audit records, backups, restore journals, and health checks. The terminal shows the current state, next action, URLs, Admin path, host details, resource paths, and live logs.
- **Own the deployment and the data.** Prods Community is AGPL-licensed, stores primary state in SQLite, and keeps assets and backups in explicit local roots. Release builds target Linux, Windows, and macOS behind the reverse proxy you choose.

## Who it is for

Prods is designed for product businesses that need an official catalog and RFQ channel but do not want to assemble and operate a CMS, custom database application, search service, and separate back office. It is especially suited to component, industrial, technical, and B2B catalogs where part numbers, taxonomy, specifications, documents, publication control, and inquiry context matter.

V1 deliberately does not implement checkout, pricing, inventory availability, CRM, a generic page builder, arbitrary custom JavaScript, or automated Product creation from an RFQ. Those boundaries keep catalog truth, public publishing, and buyer inquiries explicit.

## Capabilities

| Area | What Prods provides |
| --- | --- |
| Catalog operations | Current and Archived Products, arbitrary-depth categories, specifications and spec sets, dictionaries, images/documents, pagination, sorting, XLSX import/export, and bulk Publish/Hide/Archive/category/lifecycle actions |
| Website and languages | Versioned Website settings, preview/publish flow, Public Copy overrides, navigation and theme controls, a separate Admin UI locale, and ten built-in public locales |
| Public discovery | Server-rendered search and catalog pages, Product/category/manufacturer/brand/application routes, Unicode-folded search, JSON-LD, JSON, Markdown, Sitemap, robots, manifest, and `llms.txt` |
| RFQ | Multi-product requests, Requested Part intake from no-result searches, high-entropy idempotency keys, durable receipts, Admin review, and explicit optional SMTP sending |
| Access and accountability | Opaque server-side sessions, CSRF protection, capability-based roles, scoped Admin access, and records for required Admin actions |
| Operations | Liveness/readiness, runtime logs, scheduled and one-shot backup, verified restore, Recovery, Owner recovery, Maintenance, asset cleanup, and on-demand update checking |
| Distribution | Empty installation by default, optional exact-version sample data, embedded source revision/version, checksums, and Linux/Windows/macOS release artifacts |

The built-in locales are `en-US`, `zh-TW`, `zh-CN`, `ja-JP`, `ko-KR`, `de-DE`, `fr-FR`, `it-IT`, `es-ES`, and `pt-BR`. Their bundled defaults have passed key, placeholder, plural, formatting, and layout checks, but have not received professional native-language, legal, or marketing review. Review customer-facing copy before a production launch.

## Quick start

Download the files for your version from [GitHub Releases](https://github.com/herefindalex/prods/releases):

- `prods-linux-amd64`, `prods-windows-amd64.exe`, `prods-darwin-arm64`, or `prods-darwin-amd64`
- `SHA256SUMS` and `BUILD_INFO.txt`
- `LICENSE`, `LICENSE_POLICY.md`, the README files, and `THIRD_PARTY_NOTICES.md`

Verify the downloaded files before running them. On Linux:

```sh
sha256sum --check SHA256SUMS
chmod +x prods-linux-amd64
./prods-linux-amd64
```

On Windows PowerShell:

```powershell
.\prods-windows-amd64.exe
```

On Apple Silicon macOS:

```sh
chmod +x prods-darwin-arm64
./prods-darwin-arm64
```

Intel macOS uses `prods-darwin-amd64`. **The macOS binaries are currently cross-compiled candidates only: they have not been run on macOS, code-signed, or notarized. Treat macOS as unverified until native runtime, Installer, Admin, backup/recovery, and shutdown acceptance is completed.**

On a fresh data directory, the terminal shows a one-time local Installer URL. Open it, claim the installation, create the first Owner, choose the site locale and time zone, and complete installation. Prods exits after the Ready marker is committed. Restart the same executable, open the displayed Admin URL, and sign in as the Owner.

Public catalog pages start at `/`; Admin starts at `/admin`.

### Terminal console

In an interactive terminal, Prods uses a Bubble Tea console with three visual areas:

1. A one-line `Prods` product header.
2. Status and action guidance, including the Installer or Recovery URL, public URL, Admin URL, version, host, listen address, and resource paths.
3. Scrollable live logs.

Use Up/Down, `PgUp`/`PgDn`, `Home`, and `End` to inspect logs. `Ctrl+C` begins graceful shutdown. URLs use OSC 8 hyperlinks, so supported terminals make them clickable while other terminals still show copyable text. Windows Service and redirected output use newline-delimited JSON instead of the interactive interface.

### Empty or sample installation

A new installation contains no Product test data unless the Owner explicitly selects sample data.

The release-bound sample payload is compressed and compiled into every Prods executable. The installer enables it only when its `release_version` exactly matches the binary's compiled version, then validates its schema, references, duplicate identities, and payload limits before committing the Owner, minimum site settings, sample records, audit entry, and Ready marker in one SQLite transaction. No network connection or runtime sample file is required.

The binary never substitutes data from another release. Development or incorrectly packaged builds whose embedded payload does not match remain available for an empty installation. The JSON, schema, and checksum published with a release are review artifacts for the exact bytes compiled into that release; they are not downloaded by the installer.

## Portable runtime and configuration

Defaults resolve beside the executable, which makes a release easy to inspect, move, back up, or run as a service:

| Setting | Default |
| --- | --- |
| Listen address | `:8080` |
| Public Base URL | `http://127.0.0.1:8080` |
| Configuration | `prods.ini` |
| Data | `data/` |
| Backups | `backups/` |
| SQLite database | `data/prods.db` |

If the default port is occupied during first installation, Prods may select and persist the next available port. An installed or explicitly configured site fails instead of silently moving to another address.

Configuration precedence is:

```text
CLI flag > environment variable > one prods.ini file > portable default
```

Run `prods --help` for all flags. A typical host configuration is:

```ini
[prods]
listen=127.0.0.1:8080
base_url=https://catalog.example.com
data_dir=data
backup_dir=backups
trusted_proxies=127.0.0.1/32
asset_gc_grace_days=7
```

Equivalent environment variables include `PRODS_LISTEN`, `PRODS_BASE_URL`, `PRODS_DATA_DIR`, `PRODS_BACKUP_DIR`, `PRODS_CONFIG`, and `PRODS_TRUSTED_PROXIES`.

For an Internet-facing deployment, terminate HTTPS at a trusted reverse proxy, set the canonical HTTPS Base URL explicitly, and accept forwarded client information only from known proxy IPs or CIDRs. Never expose data, backup, secret, or generated-history directories through a generic file server.

## Publishing and RFQ behavior

Prods keeps Domain Truth, Website configuration, Public Copy, and generated public representations separate.

Publishing a Product schedules an immutable public representation. Its route, HTML, JSON-LD, JSON, Markdown, and public revision activate as one unit. Search, category, manufacturer, brand, and Sitemap aggregates may converge afterward, but they do not expose a revoked Product or publish links to unavailable content. Website-wide URL-pattern and prefix changes use an atomic site epoch switch.

RFQ submission never creates a Product, invents a price or availability state, or sends email automatically. SMTP is optional. An authorized Admin user chooses when to send, and Prods records the outcome for each recipient.

## Backup, recovery, and maintenance

Create and monitor backups in Admin, or run a one-shot backup:

```sh
./prods-linux-amd64 --backup-now
```

An offline restore selects a verified backup ID or path and exits after verified roll-forward:

```sh
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

By default, restoring a live site first requires a verified pre-restore backup. `--allow-restore-without-prebackup` is an explicit host-authorized exception for a current state that cannot be backed up.

If the database is damaged or a prepared operation was interrupted, Prods enters Recovery Required instead of initializing over existing data. The terminal presents a separate expiring Recovery URL. Recovery does not depend on the normal database, Admin session, Public artifacts, or Custom CSS.

If the Owner password is unavailable, create a one-time local recovery link without reopening Installer:

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

Manual Maintenance starts and ends from Admin System Health. It returns HTTP 503 for new Public and RFQ admission while keeping Admin, Recovery, system assets, and liveness available.

## Optional integrations and updates

SMTP credentials and Google Search Console OAuth secrets are loaded from private files rather than stored as bearer values in SQLite. Secret files stay under `data/secrets` so backup and restore can treat them as a required root.

IndexNow and Google Search Console submission are optional and require a public HTTPS Base URL. A successful submission receipt means the provider accepted the request; it does not mean a page was indexed. Public rendering and machine-readable outputs continue to work without either provider.

Admin System Health checks the latest stable GitHub release only when requested. If a newer semantic version includes a binary for the running platform, Admin presents a direct download link. Network errors, private or unavailable repositories, invalid release metadata, and missing platform assets remain non-blocking.

## Architecture

```text
Browser / crawler
       │
       ▼
single Prods Go process
  ├─ Public: semantic HTML + JSON-LD + JSON + Markdown + Sitemap/manifest/llms.txt
  ├─ Admin: embedded React + headless Refine + Ant Design application
  ├─ System: embedded Installer / Recovery / Maintenance application
  ├─ application services: catalog, RFQ, import/export, publication, backup/restore
  ├─ SQLite: authoritative state, migrations, durable jobs, receipts, audit
  └─ private roots: immutable assets, secrets/config, public-state, backups
```

Key boundaries:

- The executable is the only required application process. Node.js is a build dependency, not a user runtime dependency.
- SQLite is the primary database. Prods owns migrations and coordinates writes through bounded admission.
- Public output comes from an explicit `PublicView` allowlist rather than Admin DTOs or database rows.
- Product publication and revocation use durable intent and generated representations; success is never reported without durable evidence.
- RFQ idempotency stores the key, canonical payload hash, RFQ ID, and replayable receipt in one transaction.
- Restore uses a durable journal. Before `prepared` it can be abandoned safely; after `prepared` it only rolls forward through the same operation with per-root evidence.
- Admin uses opaque server-side sessions, server-side authorization, same-origin CSRF protection, and trusted Host/proxy settings.

## Build and verify from source

Required tools:

- Go 1.27.1
- Node.js 24
- pnpm 10.28.1, pinned by `packageManager`

```sh
pnpm --dir web/ui install --frozen-lockfile
python3 scripts/check-license-policy.py
pnpm --dir web/ui typecheck
pnpm --dir web/ui test
pnpm --dir web/ui build
go test ./... -count=1
go test -tags poc ./... -count=1 -timeout=5m
go vet ./...
CGO_ENABLED=0 go build -trimpath -o prods ./cmd/prods
```

The frontend build writes embedded assets under `internal/webapp/static/`; commit those generated files with their source changes.

Build a complete Linux amd64, Windows amd64, macOS Intel, and macOS Apple Silicon release with an embedded version, matching sample data, source metadata, notices, and checksums:

```sh
./scripts/build-release.sh v0.6.8
```

Output is written to `dist/<version>/`. Root [`VERSION`](VERSION) is the release source of truth; the script rejects any different argument, and GitHub Actions rejects a different tag. The same version is embedded in every binary, written into the sample payload and `BUILD_INFO.txt`, and used for the artifact directory and GitHub Release tag. Cross-builds prove compilation only. Windows must be tested on Windows before claiming Windows runtime verification; both macOS artifacts remain explicitly untested until native macOS acceptance is recorded.

## GitHub Actions and releases

`.github/workflows/ci.yml` runs on every push and pull request. It installs pinned frontend dependencies, runs typecheck and frontend tests, rebuilds embedded assets and rejects drift, runs the normal and PoC Go suites, runs `go vet`, verifies the generated sample contract, cross-builds Linux amd64, Windows amd64, macOS Intel, and macOS Apple Silicon binaries, and uploads short-lived CI artifacts.

Pushing a `v*` tag starts `.github/workflows/release.yml`. The workflow repeats source validation, calls `scripts/build-release.sh` with the exact tag, verifies `SHA256SUMS`, uploads a workflow artifact, and publishes every release file to the matching GitHub Release. Tags containing a hyphen are marked as prereleases and are excluded from the Admin latest-stable update path.

The release script requires a clean tracked working tree and records the version and source revision in the binaries and `BUILD_INFO.txt`. It regenerates the tracked sample payload and schema checksum and requires an exact match before publishing.

## License and contributions

Prods Community is licensed under the [GNU Affero General Public License v3.0 only](LICENSE), identified as `AGPL-3.0-only`. Network users are entitled to the corresponding source for the running version under that license.

The current release closure contains only third-party components under MIT, BSD, Apache-2.0, ISC/0BSD, or public-domain terms. The audited graphs contain no third-party GPL, AGPL, LGPL, SSPL, BSL, Commons Clause, PolyForm, non-commercial, or no-derivatives dependency. See [LICENSE_POLICY.md](LICENSE_POLICY.md) for the full scope, build-only exceptions, future closed-source conditions, and automated gate; retained notices and license texts are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

Permissive third-party licenses do not prevent a future separately licensed proprietary edition. The central condition is ownership of Prods contributions: before merging material external contributions, the project needs a Contributor License Agreement that explicitly grants the accepting legal entity the rights required to relicense those contributions in open-source and proprietary products. A commit sign-off or DCO alone is not a substitute. Product data, images, datasheets, RFQs, backups, and other customer content are not automatically relicensed as Prods source code.
