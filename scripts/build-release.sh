#!/bin/sh
set -eu

version=${1:-}
case "$version" in
  ""|*[!0-9A-Za-z._+-]*)
    echo "usage: $0 <release-version>" >&2
    echo "release-version may contain only letters, digits, dot, underscore, plus, and hyphen" >&2
    exit 2
    ;;
esac

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
required_go=go1.27.1
actual_go=$(go env GOVERSION)
if [ "$actual_go" != "$required_go" ]; then
	echo "release build requires $required_go; found $actual_go" >&2
	exit 1
fi

if ! source_revision=$(git -C "$repo_root" rev-parse --verify HEAD 2>/dev/null); then
	echo "release build requires a git source revision" >&2
	exit 1
fi
if [ -n "$(git -C "$repo_root" status --porcelain --untracked-files=normal)" ]; then
	echo "release build requires a clean tracked working tree" >&2
	exit 1
fi

dist_root="$repo_root/dist"
destination="$dist_root/$version"
staging="$dist_root/.$version.incomplete.$$"
if [ -e "$destination" ] || [ -e "$staging" ]; then
  echo "release output already exists: $destination" >&2
  exit 1
fi

mkdir -p "$staging"
cleanup() {
  if [ -d "$staging" ]; then
    find "$staging" -type f -exec chmod u+w {} \; 2>/dev/null || true
    rm -rf -- "$staging"
  fi
}
trap cleanup EXIT HUP INT TERM

cd "$repo_root"
pnpm --dir web/ui build
if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
	echo "web build changed tracked files; commit generated assets before a release build" >&2
	exit 1
fi

go run ./cmd/prods-sample -release-version "$version" -output "$staging/prods-sample-data-v1.json"
(
	cd "$staging"
	sha256sum prods-sample-data-v1.json > prods-sample-data-v1.json.sha256
)

ldflags="-s -w -X main.applicationVersion=$version -X main.sourceRevision=$source_revision"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$staging/prods-linux-amd64" ./cmd/prods
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$staging/prods-windows-amd64.exe" ./cmd/prods
cp LICENSE LICENSE_POLICY.md README.md README-zh_TW.md README-zh_CN.md THIRD_PARTY_NOTICES.md "$staging/"
{
	printf 'Version: %s\n' "$version"
	printf 'Source revision: %s\n' "$source_revision"
	printf 'Go: %s\n' "$actual_go"
} > "$staging/BUILD_INFO.txt"

(
	cd "$staging"
	sha256sum BUILD_INFO.txt LICENSE LICENSE_POLICY.md README.md README-zh_TW.md README-zh_CN.md THIRD_PARTY_NOTICES.md prods-linux-amd64 prods-windows-amd64.exe prods-sample-data-v1.json prods-sample-data-v1.json.sha256 > SHA256SUMS
)
mv "$staging" "$destination"
trap - EXIT HUP INT TERM
printf 'release artifacts: %s\n' "$destination"
