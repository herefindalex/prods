# Prods installer sample data

Release builds generate `prods-sample-data-v1.json` from the deterministic synthetic reference catalog. The file is a GitHub Release asset and is never embedded in the Prods runtime binary. Its SHA-256 companion is `prods-sample-data-v1.json.sha256`.

The format is defined by `schema-v1.json`. `schema_version` changes only for an incompatible file-format change. `release_version` must exactly match the running binary's compiled release version and the GitHub Release tag. `dataset_version` identifies the synthetic data revision independently of the application release. The installer rejects unknown fields, checksum mismatches, a different release version, duplicate IDs or current identities, invalid parent ordering, and invalid domain values.

The v1 installer payload contains the reference taxonomy and core Product records. Products marked `published` create normal publication intents; generated public representations converge after the installed site restarts in Normal mode. The richer test-only reference catalog remains the source for specification, translation, document, and expected-result verification and is not linked into `cmd/prods`.

Generate a payload locally:

```sh
go run ./cmd/prods-sample -release-version v1.0.0 -output /tmp/prods-sample-data-v1.json
```
