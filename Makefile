# Thin wrappers over go commands and scripts/; every target works without make too.
LINT := go tool -modfile=tools/go.mod golangci-lint
.DEFAULT_GOAL := help

.PHONY: help test lint fmt check proto

help: ## List targets
	@awk 'BEGIN{FS=":.*## "} /^[a-z0-9-]+:.*## /{printf "  %-8s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: ## Run the tests, the conformance suite's own among them
	go test -count=1 ./...

lint: ## Run golangci-lint (pinned in tools/go.mod)
	$(LINT) run ./...

fmt: ## Format code with gofumpt via golangci-lint
	$(LINT) fmt ./...

check: ## Test, vet and lint for every platform, check the contract, scan for keys (what CI runs)
	scripts/check.sh

proto: ## Regenerate proto/modulev1 from proto/mindseye
	go run github.com/bufbuild/buf/cmd/buf@v1.73.0 generate
