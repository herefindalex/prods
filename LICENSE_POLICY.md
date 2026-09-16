# Dependency license policy and audit

This document records the engineering license review for Prods Community and the dependency constraints required to preserve a future separately licensed proprietary distribution. It does not grant a proprietary license to Prods Community.

## Distribution goals

Prods Community is distributed under `AGPL-3.0-only`. The project may later offer a separate commercial or proprietary license only for code whose copyright owner has the right to relicense it.

Third-party components in a release must permit all of these uses:

- distribution in an AGPL-3.0-only Community build;
- commercial use;
- inclusion in a separately licensed closed-source build;
- modification and redistribution, subject to retained notices and any reviewed component-level obligations.

MIT, ISC, 0BSD, BSD-2-Clause, BSD-3-Clause, and Apache-2.0 dependencies are normally acceptable. Public-domain code is acceptable when provenance is clear. MPL-2.0, LGPL, EPL, CDDL, and similar scoped-copyleft licenses require an explicit review before entering a release binary or browser bundle. GPL, AGPL, SSPL, Business Source License, Commons Clause, PolyForm, non-commercial, and no-derivatives dependencies are not accepted in a proprietary release closure without a separate written licensing decision.

## Audited dependency snapshot

Audit date: 2026-09-16. Exact inputs and expected counts are recorded in [`licenses/audit.json`](licenses/audit.json).

| Scope | Result |
| --- | --- |
| Linux/Windows/macOS Go release closure | 37 external modules: MIT, BSD-3-Clause, and Apache-2.0 only |
| Full Go module graph | 60 external modules; one reviewed MPL-2.0 module is used by build/test paths and is verified absent from both release closures |
| Embedded production UI graph | 106 package records: MIT, BSD-3-Clause, and 0BSD only |
| Full frontend build/test graph | 198 package records: production licenses plus Apache-2.0, ISC, and one CC-BY-4.0 build-data package |
| GitHub Actions and release CLI | `checkout`, `setup-go`, `action-setup`, `setup-node`, `upload-artifact`, and GitHub CLI are MIT-licensed |
| Fonts and remote browser assets | No bundled web font or remotely loaded font dependency was found |

The MPL-2.0 module is `github.com/hashicorp/golang-lru/v2@v2.0.7`. It is reached through modernc build/test packages and is not linked into the Linux, Windows, or macOS release binaries. CI fails if it enters any release closure.

The CC-BY-4.0 component is `caniuse-lite@1.0.30001810`, used as browser-compatibility build data. It is absent from the production package set and is not shipped as a runtime package. Its attribution remains available through the pinned source package and lockfile.

No third-party GPL, AGPL, LGPL, SSPL, Business Source License, Commons Clause, PolyForm, non-commercial, or no-derivatives dependency was found in the audited graphs.

## Release obligations

Every binary release must include:

- Prods `LICENSE` and the README files;
- `THIRD_PARTY_NOTICES.md`, including retained copyright, permission, warranty, NOTICE, and public-domain statements for linked Go modules and bundled browser packages;
- `BUILD_INFO.txt` and `SHA256SUMS` tying the artifacts to the exact version and source revision;
- corresponding Prods source availability required by AGPL-3.0-only for the Community build.

Apache-2.0 notices and patent terms remain intact. The SQLite amalgamation embedded through modernc is public domain; modernc wrapper code and its listed third-party portions retain their BSD notices. The release process must not remove these notices when packaging a proprietary build.

## Preserving a future proprietary edition

Permissive third-party dependencies do not prevent a closed commercial distribution. The main ownership constraint is Prods code itself: accepting an external contribution under AGPL-3.0-only alone does not automatically give the project owner the right to relicense that contributor's copyright.

Before accepting material external contributions, the project must adopt a Contributor License Agreement that explicitly grants the accepting legal entity the rights needed to reproduce, modify, sublicense, and relicense the contribution in open-source and proprietary products. The accepting legal entity, CLA text, contributor records, and authority to license the Prods name must be established before relying on dual licensing. Until that process exists, material external contributions must not be merged.

## Enforcement

Run the license gate after installing frontend dependencies:

```sh
python3 scripts/check-license-policy.py
```

The gate checks audited hashes for `go.mod`, `go.sum`, `package.json`, and `pnpm-lock.yaml`; the Go release closure for Linux, Windows, and both macOS architectures; npm license-group counts; the reviewed build-only exception; and notice coverage for every release module and production package. Any dependency input change requires a new review and a deliberate update to `licenses/audit.json` and `THIRD_PARTY_NOTICES.md`.

This is an engineering compatibility audit. Counsel should review the final CLA, trademark policy, and proprietary customer license before the first closed-source commercial distribution.
