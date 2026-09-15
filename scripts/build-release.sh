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

ldflags="-s -w -X main.applicationVersion=$version"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$staging/prods-linux-amd64" ./cmd/prods
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$ldflags" -o "$staging/prods-windows-amd64.exe" ./cmd/prods
cp LICENSE README.md THIRD_PARTY_NOTICES.md "$staging/"

(
  cd "$staging"
  sha256sum LICENSE README.md THIRD_PARTY_NOTICES.md prods-linux-amd64 prods-windows-amd64.exe > SHA256SUMS
)
mv "$staging" "$destination"
trap - EXIT HUP INT TERM
printf 'release artifacts: %s\n' "$destination"
