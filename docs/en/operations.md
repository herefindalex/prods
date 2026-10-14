# Operations and production evidence

English | [繁體中文](../zh-TW/operations.md) | [简体中文](../zh-CN/operations.md)

[README](../../README.md) · [Architecture](architecture.md) · [Engineering case study](engineering-case-study.md)

## Production status

Reviewed on 2026-09-17 against the working tree (`VERSION`: `v0.6.8`). The repository contains a runnable application, release packaging, operational controls and automated tests. These are implementation evidence. This documentation review did not verify a live production endpoint, its deployed revision, real customer workload, production TLS configuration or an off-host restore exercise. “Production” here describes an intended operator-managed installation serving real catalog/RFQ data; it is not an asserted deployment milestone.

Source links and test files below let readers inspect the mechanisms. A test's presence is not a claim that it passed on every platform, and workflow configuration is not a successful CI run. The checkout includes ongoing changes; it is not itself a clean release artifact.

## Runtime and deployment boundary

The release model is one platform-specific Go binary, embedded frontend assets, local SQLite and managed files. [`main.go`](../../cmd/prods/main.go) takes an OS-level database ownership lock before opening normal or one-shot operations. [`service_linux.go`](../../internal/platform/service_linux.go) generates an optional systemd unit; [`service_windows.go`](../../internal/platform/service_windows.go) implements Windows service integration. A service definition is not evidence of service-manager acceptance on a real host.

Internet-facing use expects an operator-supplied TLS reverse proxy and an explicit HTTPS Base URL. Prods checks request authority and supports configured trusted proxy addresses. The repository does not supply evidence of a specific live proxy/TLS topology. A reverse proxy must forward to the application, not expose data, backups, secrets or generated-history directories. See [`proxy_trust.go`](../../internal/webapp/proxy_trust.go).

Defaults are resolved relative to the executable:

| Setting | Default / role |
| --- | --- |
| Listen | `:8080`; this is not a loopback-only default |
| Base URL | `http://127.0.0.1:8080` |
| Config | `prods.ini` |
| Data / DB | `data/` / `data/prods.db` |
| Backups | `backups/` |
| Managed data subdirectories | Assets, work, generated output, secrets, control journals and logs |

Precedence is CLI flag → environment variable → one config file → defaults. [`hostconfig/config.go`](../../internal/hostconfig/config.go) defines the flags and validation. Use the README's example and the actual binary's `--help`; advanced paths and credentials should not be inferred from a developer's local configuration.

## How to start an isolated local installation

Use a Linux amd64 release binary and a new directory, or the source-build commands in the [README](../../README.md). The following Linux shell example assumes the binary is in the current directory and port 18080 is free:

```sh
demo_dir="$(mktemp -d)"
cp ./prods-linux-amd64 "$demo_dir/prods"
chmod +x "$demo_dir/prods"
"$demo_dir/prods" --listen 127.0.0.1:18080 --base-url http://127.0.0.1:18080
```

1. Open the one-time Installer URL printed by the terminal. Create the Owner and complete installation. An empty catalog is valid; sample data is optional and bound to the binary's release version.
2. After Prods exits, repeat the last command. Sign in at `http://127.0.0.1:18080/admin`; inspect the public site at `/`.
3. Check `http://127.0.0.1:18080/health/live` and `/health/ready`. Readiness must be evaluated separately from process liveness. Stop with `Ctrl+C` when finished.

If the explicit port is occupied, choose another and change both listen and Base URL. If startup reports damaged/unrecognized data, retain the directory and use Recovery; do not treat it as a request for reinstallation. Installer and normal-startup behavior are exercised by [`main_test.go`](../../cmd/prods/main_test.go), [`installer tests`](../../internal/webapp/installer_test.go) and [`installation tests`](../../internal/storage/sqlite/installation_test.go).

## How to back up and restore

For a running site, use Admin Backup or the scheduler. For the command-line one-shot operations below, first stop the service/foreground instance and use the same executable/config/data paths. The ownership lock prevents a second process from sharing the instance.

```sh
./prods-linux-amd64 --backup-now
./prods-linux-amd64 --restore-backup <backup-id-or-path>
```

Replace the restore placeholder with a selected backup. Restore changes persistent state; the second command is an alternative operation, not a required follow-up to every backup. It normally requires a verified pre-restore backup of the current site. The host-authorized `--allow-restore-without-prebackup` exception exists for a state that cannot be backed up; it is not the default procedure.

Backup pairs a consistent database snapshot with its referenced assets, configured roots and files, and an independent manifest. The manifest records content verification and read-only application separately. Secrets make backups sensitive. A checksum/read-only flag does not establish encryption, tamper resistance or survival of host loss. External secret requirements must also be satisfied.

Restore stages and verifies roots, persists `prepared`, then rolls the same operation forward using per-root evidence. After final verification, restart normally and check readiness, Admin access, public pages and required assets. If recovery reports missing evidence or mismatched roots, retain the journal and source backup; do not delete the journal to force normal startup. See [`backup.go`](../../internal/recovery/backup.go), [`restore.go`](../../internal/recovery/restore.go), [`restore fault tests`](../../internal/recovery/restore_fault_test.go) and [`backup/restore CLI tests`](../../cmd/prods/backup_restore_test.go).

## How to recover Owner access

With the normal instance stopped, use the same configuration and an existing active Owner's address:

```sh
./prods-linux-amd64 --recover-owner owner@example.com
```

The command prints an expiring one-time link and exits. Restart Prods normally, then open the link before it expires. Keep the link private. The command requires a completed site database; it does not reopen Installer or repair an unreadable database. Evidence: [`main.go`](../../cmd/prods/main.go), [`identity_store.go`](../../internal/storage/sqlite/identity_store.go) and [`user lifecycle tests`](../../internal/webapp/user_lifecycle_test.go).

## Startup, upgrades, shutdown and diagnosis

- **Startup:** durable inspection distinguishes fresh, installing, ready, upgrade and recovery states. Incomplete installation receives a new claim; successful installation requires restart.
- **Migrations:** versioned SQL and migration history ship in the binary. Upgrade startup creates/verifies a backup and records migration progress before normal operation. An interrupted operation is reconciled or enters Recovery, not silently accepted.
- **Shutdown:** new work is drained before shutdown and component closure. HTTP shutdown has a timeout and can force-close; this is not a guarantee that every client request completes under all failures. [`shutdown_test.go`](../../cmd/prods/shutdown_test.go) covers bounded behavior.
- **Maintenance:** new Public/RFQ admission returns 503 while permitted Admin, Recovery, system assets and liveness remain available. This is a controlled outage, not high availability.
- **Diagnosis:** JSON runtime logs and the interactive terminal expose runtime state; Admin exposes jobs and health. [`runtime_log.go`](../../internal/platform/runtime_log.go) and [`runtime_health.go`](../../internal/platform/runtime_health.go) implement local mechanisms. No external monitoring/on-call deployment is established here.
- **Secrets/integrations:** normal-site SMTP and Google credentials are read from private files. Optional mail/indexing operations are distinct from catalog serving; a provider receipt does not prove indexing or exactly-once mail delivery. Inspect [`maildelivery`](../../internal/maildelivery/manager.go) and [`searchnotify`](../../internal/searchnotify/searchnotify.go).

## Build, release and verification scope

[`ci.yml`](../../.github/workflows/ci.yml) defines frontend typecheck/tests, embedded-asset drift checks, Go normal/PoC tests, vet, license/sample checks and cross-builds. [`release.yml`](../../.github/workflows/release.yml) validates a `v*` tag and publishes release files; it does not deploy a running site. [`build-release.sh`](../../scripts/build-release.sh) checks `VERSION`, Go version, source revision, clean working tree and exact sample artifacts, then emits binaries, build information and checksums.

Linux amd64, Windows amd64 and macOS amd64/arm64 are build targets. Windows/macOS native runtime, services, signing and platform-specific recovery require separate evidence for the exact artifact. Linux tests cannot settle those questions. POC-01 renderer feasibility and POC-03 publication protocol correctness are different gates; a rendering smoke test does not establish publication correctness or production readiness.

The full source-verification commands remain in the [README](../../README.md). Frontend builds rewrite embedded files, and release packaging requires a clean checkout. Run them in a suitable checkout rather than overwriting unrelated in-progress work merely to validate documentation.

## Current Operational Boundaries

| Repository evidence | Boundary of the claim |
| --- | --- |
| Instance locking and one-process runtime | No multi-node coordination, failover or zero-downtime upgrade guarantee. |
| Serialized SQLite writes, import tests | No measured production capacity or zero RFQ wait during atomic imports. |
| Publication admission and protocol tests | Tests cover stated scenarios; previously admitted transfers may finish after revocation. |
| Receipts and audit records | Durable application evidence, not an exactly-once guarantee for external services. |
| Backup manifests and restore journals | No verified off-host backup policy, recovery-time/recovery-point objective or power-loss guarantee for every filesystem. |
| Sessions, CSRF, capability checks and file validation | Implemented controls, not security certification or a comprehensive security guarantee. |
| Ten built-in locale resources | No professional native-language, legal or marketing review claimed. |
| Build/release workflows and sample catalog | No customer counts, business traction, live traffic, SLA or actual deployment inferred from fixtures or automation. |

## Evidence still needed for a production account

An operator can supplement this repository with the deployed version/source revision, host/platform, service/proxy configuration, TLS validation, persistent-root mapping, backup retention/off-host destination, a dated restore exercise and measured workload/incident evidence. Keep credentials and customer data private. Until that evidence exists, describe the implementation and its tests rather than an unverified production success story.

## Numeric specification search (backend)

This section documents the `v0.7.0` backend reviewed on 2026-10-14. Earlier sections and the architecture/case-study guides retain their historical `v0.6.8` review scope. Numeric profile editing, shared-source/adoption controls and Public numeric filter UI are deferred; existing text search is separate.

### Configure the candidate backend

`bleve` is the default. The flag `--spec-search-backend`, environment variable `PRODS_SPEC_SEARCH_BACKEND` and INI key `[prods] spec_search_backend` accept `bleve` or `sqlite` with CLI > ENV > INI > default precedence. Select one of these alternatives and restart using the same site paths:

```ini
[prods]
spec_search_backend=bleve
```

```sh
PRODS_SPEC_SEARCH_BACKEND=sqlite ./prods-linux-amd64
./prods-linux-amd64 --spec-search-backend sqlite
```

Bleve is embedded in the Go process and writes a private, rebuildable candidate index at `<data_dir>/generated/search/specs`. SQLite remains the primary store; `sqlite` uses its derived Spec-ID projection instead. Never expose the generated/search directory through a filesystem server. Candidate replacement/rebuild runs in the publication worker after active public state changes.

If candidates fail, are incomplete, or do not match the admitted inventory/revisions, queries scan the complete active public inventory and evaluate it exactly. This preserves results and counts at additional scan cost; it is not a latency guarantee. Logs report candidate-index failures. Later worker ticks retry refresh on an opened adapter; a Bleve adapter that failed its initial open is not reopened by those ticks. Fix its filesystem/ownership/resource cause and restart, or restart with `sqlite`. `SpecSearchDiagnostics` and `RebuildSpecSearchIndex` are internal methods, with no Admin/Public diagnostics or rebuild endpoint. An unhealthy authoritative public index still returns 503.

### Prepare and adopt a numeric fact

A Product + Spec has one selected shared Raw source. `source_origin_locale` records the source provenance; it does not create a different numeric fact per public language. Localized display text stays separate. Migration 19 retains legacy Raw/history, selects the source only where there is exactly one row per Product + Spec, and leaves multiple-source legacy groups unselected until an explicit command. It creates no numeric profiles or adopted facts from old display units/history. Published Current Products receive a new source revision and publication intent; the previous valid public activation remains until replacement. The bundled synthetic sample likewise provides Raw specifications, not configured profiles/adopted numeric facts.

Use a legitimate same-origin Admin login and its private cookie jar (`admin_cookie_jar`); obtain `admin_csrf` from the signed-in Admin page's `csrf-token` meta value. Set `base` to the configured Base URL and the ID variables to existing domain IDs. Admin reads require `catalog.view`; writes require `catalog.edit`, the session cookie and `X-CSRF-Token`. These are browser Admin commands, not external write tokens. The following bodies are illustrative files: substitute actual IDs and the revisions observed when opening the operation. Each mutation example is a separate operation, not a script to run blindly in sequence.

| Method / path | Capability / body |
| --- | --- |
| `PUT /admin/api/specs/{id}/numeric-profile` | `catalog.edit`; `expected_revision`, `profile` |
| `GET /admin/api/products/{id}/spec-values` | `catalog.view`; inspect stored Raw sources |
| `POST /admin/api/products/{id}/spec-source` | `catalog.edit`; `spec_id`, `spec_value_id`, `expected_product_revision`, `expected_source_revision`, `expected_source_selection_revision` |
| `GET /admin/api/spec-values/{id}/normalization-context` | `catalog.view`; frozen Raw, profile, `expected`, `selection`, `source_selection` |
| `POST /admin/api/spec-values/{id}/normalize` | `catalog.edit`; `expected` |
| `POST /admin/api/spec-values/{id}/normalized` | `catalog.edit`; `expected`, `value_json` (JSON encoded as a string) |
| `POST /admin/api/spec-values/{id}/normalization/automatic` | `catalog.edit`; `expected`, `candidate_id` |

Example `profile-command.json` for an operating-voltage Spec (use the Spec definition's actual `revision` as `expected_revision`):

```json
{
  "expected_revision": 1,
  "profile": {
    "quantity": "voltage",
    "semantic_role": "operating_support",
    "allowed_shapes": ["scalar", "discrete_set", "interval_set"],
    "allowed_operators": ["contains_point", "contains_range", "overlaps_range"],
    "canonical_unit": "V",
    "declared_input_unit": "V",
    "numeric_format": "decimal-dot",
    "schema_version": 1,
    "parser_version": "electrical-parser-1",
    "unit_registry_version": "electrical-1"
  }
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json' -X PUT   "$base/admin/api/specs/$spec_id/numeric-profile" --data-binary @profile-command.json
curl -b "$admin_cookie_jar" \
  "$base/admin/api/products/$product_id/spec-values"
curl -b "$admin_cookie_jar" \
  "$base/admin/api/spec-values/$spec_value_id/normalization-context"
```

If the context has no selected shared source, select one first with `source-command.json`. Use context `expected.product_revision`, `expected.source_revision` and `source_selection.revision` for the three expectations, then open a new normalization context after success:

```json
{
  "spec_id": "spec_input_voltage",
  "spec_value_id": "spv_selected_source",
  "expected_product_revision": 7,
  "expected_source_revision": 1,
  "expected_source_selection_revision": 0
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/products/$product_id/spec-source" --data-binary @source-command.json
```

Save the following as `normalize-command.json`, replacing all five values with the entire `expected` object returned by `normalization-context`: `product_revision`, `source_revision`, `spec_semantic_version`, `profile_revision`, and `selection_revision`. The first four must be positive; the selection revision may be zero. The write transaction rechecks every expectation and the selected shared source.

```json
{
  "expected": {
    "product_revision": 7,
    "source_revision": 1,
    "spec_semantic_version": 2,
    "profile_revision": 1,
    "selection_revision": 0
  }
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/spec-values/$spec_value_id/normalize" --data-binary @normalize-command.json
```

Normalization may adopt a valid automatic result, record a candidate when manual preference is active, or return HTTP 200 with `state: "failed"` and `reason_code` for unparseable Raw. It does not overwrite a manually adopted selection. Manual adoption uses a validated, canonical-unit `Value` encoded in `value_json`; for the voltage profile above, `manual-command.json` can be:

```json
{
  "expected": {"product_revision": 7, "source_revision": 1, "spec_semantic_version": 2, "profile_revision": 1, "selection_revision": 0},
  "value_json": "{\"schema_version\":1,\"quantity\":\"voltage\",\"semantic_role\":\"operating_support\",\"shape\":\"scalar\",\"unit\":\"V\",\"unit_registry_version\":\"electrical-1\",\"scalar\":\"5\",\"context\":{\"kind\":\"unconditional\"}}"
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/spec-values/$spec_value_id/normalized" --data-binary @manual-command.json
```

To replace manual preference with a current automatic candidate, explicitly choose its server-recorded ID and a newly reviewed context in `automatic-command.json`; an arbitrary, stale or incompatible candidate is rejected:

```json
{
  "expected": {"product_revision": 8, "source_revision": 1, "spec_semantic_version": 2, "profile_revision": 1, "selection_revision": 1},
  "candidate_id": "nrm_current_automatic_candidate"
}
```

```sh
curl -b "$admin_cookie_jar" -H "X-CSRF-Token: $admin_csrf" \
  -H 'Content-Type: application/json'   "$base/admin/api/spec-values/$spec_value_id/normalization/automatic" --data-binary @automatic-command.json
```

Successful adoption increments Product/selection revisions and may return `publication_pending: true`. Saved or scheduled does not mean publicly active. Do not retry writes automatically. On 409, retain edits, read a new context and review the changed versions before issuing a new command.

### Query frozen public facts

`POST /api/search/specs` is a bounded read command and does not require an Admin session or Admin CSRF token. Replace the example Spec/category IDs with published IDs whose profiles allow those operators and units. The output-current example requires a separate current profile allowing `>=`:

```sh
curl -H 'Content-Type: application/json' "$base/api/search/specs" --data-binary '{
  "predicates": [
    {"spec_id":"spec_input_voltage","operator":"contains_point","value":"5","unit":"V"},
    {"spec_id":"spec_output_current","operator":">=","value":"0.5","unit":"A"}
  ],
  "page":1,"page_size":20
}'
curl -H 'Content-Type: application/json' "$base/api/search/specs" --data-binary '{
  "predicates":[{"spec_id":"spec_input_voltage","operator":"contains_range","lower":"3","upper":"5","unit":"V"}],
  "category_id":"cat_power","locale":"en-US","page":1,"page_size":20
}'
```

Requests require 1–16 predicates, positive `page`, and `page_size` of 1–100. Decimal operands are JSON strings with an explicit unit. Point operators (`contains_point`, `=`, `<`, `<=`, `>`, `>=`) use only `value`; range operators (`contains_range`, `overlaps_range`, `between`) use only `lower` and `upper`. Do not send unrelated operand keys even as `null`. Optional `category_id` includes that category and descendants; `manufacturer_id` is an exact ID scope. `locale` must be publicly enabled; omission resolves the active Site Default.

All predicates must evaluate `MATCH` against adopted facts and profiles frozen in the admitted active `PublicView`. Missing, stale, conditional or incompatible facts yield `UNKNOWN` and are excluded. Disjoint operating ranges keep their gaps; nominal/tolerance and one-sided limit claims follow their declared roles, not an inferred operating range. Candidate indexes never establish numeric truth. Exact evaluation and visibility/scope checks precede `total` and pagination in one admitted snapshot. Results are `{rows,total,page,page_size}`, ordered by Part Number then stable Product ID; an out-of-range page has empty rows and retains the exact total. Pending source/profile edits affect future publication, not the currently active frozen facts.

### Interpret API outcomes

Admin: 400 `invalid_json` covers malformed bodies/expectations and invalid controlled value JSON; 401 `unauthorized` means no valid session; 403 `forbidden` or `invalid_csrf` means capability/security rejection; 404 `not_found` includes a missing source/profile context; 409 `revision_conflict` covers stale expectations or an unselected shared source; 422 `validation_failed` covers domain/profile incompatibility. Manual unit/quantity/role/allowed-shape incompatibility is 422, while a malformed tagged Value is 400. A normalization parse failure is the successful 200 outcome described above, not a transport error.

Public search: malformed JSON/operand shape, decimal encoding or page bounds returns 400 `invalid_json`; an explicitly disabled/unknown locale returns 400 `validation_failed` (an admitted locale-policy race may instead be 400 `invalid_json`). Unknown operators or an operator/unit incompatible with a scoped frozen definition return 422 `validation_failed`. No scoped definition/fact establishes no match, rather than inventing profile semantics. Unavailable authoritative publication returns 503 `service_unavailable`; unexpected server errors return 500 `internal_error`. Maintenance intercepts the route with HTTP 503 HTML and `Retry-After: 300`; draining may return a plain-text 503 before API handling. A degraded candidate index alone triggers exact fallback, not an approximate answer.

Evidence: [`spec_numeric.go`](../../internal/webapp/spec_numeric.go), [`spec_search.go`](../../internal/webapp/spec_search.go), [`numeric profile`](../../internal/specnumeric/profile.go), [`numeric value`](../../internal/specnumeric/value.go), [`exact evaluation`](../../internal/specnumeric/predicate.go), [`search engine`](../../internal/publishing/spec_search.go), [`backend lifecycle`](../../internal/publishing/spec_search_backend.go) and [`migration 19`](../../internal/storage/sqlite/migrations.go). These links describe implementation; production scale and native Windows/macOS acceptance remain separate evidence.
