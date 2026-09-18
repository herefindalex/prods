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
