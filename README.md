# Prods

Prods is a self-hosted product catalog and RFQ system for manufacturers, distributors, and other B2B teams. It runs as one platform-specific Go binary with an embedded SQLite database and embedded web assets.

The public catalog is server rendered: core Product information, search, pagination, documents, and basic RFQ submission remain usable without JavaScript. React enhances isolated public controls. Admin and the Installer, Recovery, and Maintenance system interfaces use React and Ant Design.

## Architecture

```text
Browser / crawler
       │
       ▼
single Prods Go process
  ├─ Public: semantic HTML + JSON-LD + JSON + Markdown + Sitemap/manifest/llms.txt
  ├─ Admin: embedded React + Ant Design application
  ├─ System: embedded Installer / Recovery / Maintenance application
  ├─ application services: catalog, RFQ, import/export, publication, backup/restore
  ├─ SQLite: authoritative state, migrations, durable jobs, receipts, audit
  └─ private roots: immutable assets, secrets/config, public-state, backups
```

Important boundaries:

- The executable is the only required application process. Node.js is used to build web assets and is not a runtime dependency.
- SQLite is the primary database. Prods owns migrations and coordinates every write through bounded admission.
- Public output is generated from an explicit `PublicView` allowlist, never from Admin DTOs or raw database rows.
- A Product route, HTML, JSON-LD, JSON, Markdown, and public revision activate as one publication unit. Search, category, manufacturer, and Sitemap aggregates converge without exposing revoked data or dead links.
- Hide and Archive stop new public admission before ETag or Range handling. Requests admitted before that boundary may finish.
- RFQ submission uses a separate high-entropy idempotency key and a durable completion receipt in the same transaction as the RFQ.
- Restore uses a durable journal. Before `prepared` it can be abandoned safely; after `prepared` it only rolls forward through the same operation.

## Run a binary

Download the binary for the target platform together with `LICENSE`, `THIRD_PARTY_NOTICES.md`, and `SHA256SUMS`.

Linux:

```sh
chmod +x prods-linux-amd64
./prods-linux-amd64
```

Windows PowerShell:

```powershell
.\prods-windows-amd64.exe
```

On a fresh data directory, Prods prints a one-time local Installer URL. Open that URL, claim the installation, create the first Owner, and choose the site language/time zone. A successful installation exits so that the process can be restarted in Normal mode.

The portable defaults are resolved beside the executable:

| Setting | Default |
| --- | --- |
| Listen address | `:8080` |
| Public Base URL | `http://127.0.0.1:8080` |
| Configuration | `prods.ini` |
| Data | `data/` |
| Backups | `backups/` |

If the built-in port is occupied on the first installation, Prods may select and persist the next available port. An installed or explicitly configured site fails instead of silently moving to a different address.

After restart, open `/admin` and sign in as the Owner. Public catalog pages are available from `/` and `/search`.

## Configuration

Configuration precedence is:

```text
CLI flag > environment variable > one prods.ini file > portable default
```

Run `prods --help` for every supported flag. Common settings are:

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

For production, terminate HTTPS at a trusted reverse proxy, set the canonical HTTPS Base URL explicitly, and list only proxy IPs/CIDRs that may supply forwarded client information. Do not expose the data, backup, secret, or generated-history directories through a generic file server.

## Products, publication, and RFQ

The Admin application manages Current/Archived Products, categories, specifications, dictionaries, images/documents, XLSX imports/exports, Website versions, users/roles, RFQs, jobs, backups, traffic controls, and system health.

Publishing a Product schedules its immutable public representation. Updating a Published Product keeps the previous valid public revision until the new unit is ready. Hide and Archive revoke new access synchronously. Website URL-pattern or prefix changes use a site-wide epoch switch.

RFQ does not create Products, invent price or availability, or send email automatically. SMTP is optional and sending is an explicit authorized Admin action with per-recipient durable outcomes.

## Backups, recovery, and maintenance

Create and monitor backups in Admin, or run a one-shot backup:

```sh
./prods-linux-amd64 --backup-now
```

An offline restore selects a verified backup ID or path and exits after verified roll-forward:

```sh
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

By default, restoring a live site first requires a verified pre-restore backup. `--allow-restore-without-prebackup` is an explicit host-authorized exception for a current state that cannot be backed up.

If the database is damaged or an interrupted prepared operation must be reconciled, Prods enters Recovery Required instead of starting a new Installer. The console prints a separate, expiring Recovery URL. Recovery does not depend on the normal database, Admin session, Public artifacts, or Custom CSS.

If the Owner password is unavailable, issue a one-time local recovery link without reopening Installer:

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

Manual Maintenance can be started and ended from Admin System health. It pauses new Public/RFQ admission with HTTP 503 while keeping Admin, Recovery, system assets, and liveness available.

## Optional integrations

SMTP credentials and Google Search Console OAuth secrets are loaded from private files, not stored as bearer values in the database. Secret files must stay under `data/secrets` so that backup and restore treat them as a required root.

IndexNow and Google Search Console submission are optional. They require a public HTTPS Base URL. Submission receipts mean the provider accepted the request; they never mean that a page was indexed. Public HTML, JSON-LD, Sitemap, JSON, Markdown, manifest, and `llms.txt` do not depend on these integrations.

## Build and verify from source

Required build tools:

- Go 1.27.1
- Node.js with pnpm 10.28.1

```sh
pnpm --dir web/ui install --frozen-lockfile
npm --prefix web/ui run typecheck
npm --prefix web/ui run build
go test ./... -count=1
go test -tags poc ./... -count=1 -timeout=5m
go vet ./...
CGO_ENABLED=0 go build -trimpath -o prods ./cmd/prods
```

Build Linux and Windows amd64 release artifacts with an embedded version and checksums:

```sh
./scripts/build-release.sh 1.0.0
```

The output is written under `dist/<version>/`. A Windows cross-build proves compilation only; execute the binary on Windows before claiming Windows runtime verification.

## License and contributions

Prods Community is licensed under the [GNU Affero General Public License v3.0 only](LICENSE), identified as `AGPL-3.0-only`. Network users are entitled to the corresponding source for the running version under that license. Release artifacts must remain associated with the exact source revision from which they were built.

Third-party components retain their own licenses and notices; see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Product data, images, datasheets, RFQs, backups, and other customer content are not automatically relicensed as Prods source code.

The project requires an approved Contributor License Agreement before merging a material external contribution. A commit sign-off or DCO alone is not a substitute; the formal CLA text and accepting legal entity must be established before opening that contribution path.
