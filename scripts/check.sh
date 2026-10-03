#!/usr/bin/env bash
# What CI runs: vet, test and lint as every platform sees the code, the module contract, and a
# scan for keys. Needs only Go; buf and gitleaks run at pinned versions through go run.
set -euo pipefail
cd "$(dirname "$0")/.."

lint=$(go tool -n -modfile=tools/go.mod golangci-lint) # built for this machine, whatever GOOS says
echo "==> go test"
go test -count=1 ./...
for goos in linux darwin windows; do
	echo "==> go vet, lint ($goos)"
	GOOS=$goos go vet ./...
	GOOS=$goos "$lint" run ./...
done
scripts/protocheck.sh
scripts/keyscan.sh
echo "==> ok"
