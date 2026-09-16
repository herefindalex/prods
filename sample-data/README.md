# Prods installer sample data

[`prods-sample-data-v1.json`](prods-sample-data-v1.json) is the release-bound payload generated from the deterministic synthetic reference catalog. It is tracked here so the data is reviewable in the repository, published as a GitHub Release asset, and never embedded in the Prods runtime binary. Its SHA-256 companion is [`prods-sample-data-v1.json.sha256`](prods-sample-data-v1.json.sha256).

The format is defined by `schema-v1.json`, which is published with every GitHub Release as an auditable schema snapshot. `schema_version` changes only for an incompatible file-format change. `release_version` must exactly match the running binary's compiled release version and the GitHub Release tag. `dataset_version` identifies the synthetic data revision independently of the application release. The installer rejects unknown fields, checksum mismatches, a different release version, duplicate IDs or current identities, invalid parent ordering, and invalid domain values. Runtime validation is compiled into the binary, so it never follows a mutable schema from the repository default branch.

The v1 installer payload contains the complete synthetic catalog needed to explore the product model: 69 dictionary entries (20 manufacturers, 15 brands, 24 applications, and 5 lifecycle values, plus five built-in document types created by the installer), 58 non-system categories, 34 specification definitions, 24 SpecSets, category-to-SpecSet assignments, 1,200 Products, and their applicable specification values. Product manufacturer, brand, lifecycle, and application references are preserved during installation. Products marked `published` create normal publication intents; generated public representations converge after the installed site restarts in Normal mode.

All referenced IDs must exist in the same payload, IDs must be unique, category parents must precede children, SpecSet members must reference declared specifications, each category has at most one SpecSet, and every Product specification value must be a member of that Product category's assigned SpecSet. The installer validates these relationships again inside the same SQLite transaction that creates the Owner and Ready marker, so an invalid reference rolls back the entire installation.

Generate a payload locally:

```sh
go run ./cmd/prods-sample -release-version v0.6.6 -output /tmp/prods-sample-data-v1.json
```

Before publishing, `scripts/build-release.sh` regenerates the payload and checksum, then compares them byte-for-byte with the files in this directory. A stale payload, schema, checksum, version, or nondeterministic generator stops the release.
