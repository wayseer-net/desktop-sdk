#!/usr/bin/env bash
# Lints the module contract, fails if its generated Go is stale, and, from the first tag on,
# fails on a change that breaks the wire since the last tag.
set -euo pipefail
cd "$(dirname "$0")/.."

buf() { go run github.com/bufbuild/buf/cmd/buf@v1.73.0 "$@"; }

echo "==> module contract"
buf lint
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
buf generate --output "$out"
if ! diff -r "$out/proto/modulev1" proto/modulev1; then
	echo "protocheck: proto/modulev1 is stale; run make proto" >&2
	exit 1
fi
if last=$(git describe --tags --abbrev=0 2>/dev/null); then
	buf breaking --against ".git#tag=$last"
	echo "protocheck: the wire is unbroken since $last"
else
	echo "protocheck: no tag yet, so nothing to break"
fi
