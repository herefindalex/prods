#!/usr/bin/env python3
"""Fail when dependency inputs or declared npm license groups drift from the audit."""

from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
AUDIT_PATH = ROOT / "licenses" / "audit.json"


def fail(message: str) -> None:
    raise SystemExit(f"license policy check failed: {message}")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def output(*args: str, env: dict[str, str] | None = None) -> str:
    return subprocess.check_output(args, cwd=ROOT, env=env, text=True)


def npm_licenses(*extra: str) -> dict[str, list[dict[str, object]]]:
    env = os.environ.copy()
    env["NODE_NO_WARNINGS"] = "1"
    raw = output("pnpm", "--dir", "web/ui", "licenses", "list", *extra, "--json", env=env)
    value = json.loads(raw)
    if "error" in value:
        fail(f"pnpm could not enumerate licenses: {value['error']}")
    return value


def group_counts(value: dict[str, list[dict[str, object]]]) -> dict[str, int]:
    return {license_name: len(packages) for license_name, packages in sorted(value.items())}


def module_set(goos: str) -> set[str]:
    env = os.environ.copy()
    env.update({"GOOS": goos, "GOARCH": "amd64", "CGO_ENABLED": "0"})
    raw = output(
        "go",
        "list",
        "-deps",
        "-f",
        "{{with .Module}}{{.Path}}{{end}}",
        "./cmd/prods",
        env=env,
    )
    return {line for line in raw.splitlines() if line and line != "prods"}


def main() -> None:
    audit = json.loads(AUDIT_PATH.read_text(encoding="utf-8"))
    for relative, expected in audit["inputs"].items():
        actual = sha256(ROOT / relative)
        if actual != expected:
            fail(f"{relative} changed; complete a new dependency-license review and update licenses/audit.json")

    modules = {
        line.split()[0]
        for line in output("go", "list", "-m", "all").splitlines()
        if line and not line.startswith("prods")
    }
    expected_all = audit["go"]["all_external_modules"]
    if len(modules) != expected_all:
        fail(f"Go module graph has {len(modules)} external modules; audited value is {expected_all}")

    release_modules = module_set("linux") | module_set("windows")
    expected_release = audit["go"]["release_external_modules"]
    if len(release_modules) != expected_release:
        fail(f"release closure has {len(release_modules)} Go modules; audited value is {expected_release}")
    if "github.com/hashicorp/golang-lru/v2" in release_modules:
        fail("reviewed build-only MPL-2.0 module entered the release binary closure")

    all_npm = npm_licenses()
    production_npm = npm_licenses("--prod")
    all_counts = group_counts(all_npm)
    production_counts = group_counts(production_npm)
    if all_counts != audit["npm"]["all_license_groups"]:
        fail(f"npm license groups changed: {all_counts}")
    if production_counts != audit["npm"]["production_license_groups"]:
        fail(f"production npm license groups changed: {production_counts}")

    notice = (ROOT / "THIRD_PARTY_NOTICES.md").read_text(encoding="utf-8")
    missing_go = sorted(module for module in release_modules if module not in notice)
    production_packages = {
        str(package["name"])
        for packages in production_npm.values()
        for package in packages
    }
    missing_npm = sorted(package for package in production_packages if package not in notice)
    if missing_go or missing_npm:
        fail(f"THIRD_PARTY_NOTICES.md is missing Go={missing_go} npm={missing_npm}")

    print(
        "license policy: OK "
        f"(Go release modules={len(release_modules)}, npm production records={sum(production_counts.values())})"
    )


if __name__ == "__main__":
    main()
