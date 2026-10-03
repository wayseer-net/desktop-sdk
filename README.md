# Wayseer SDK

The SDK for Wayseer Desktop modules: everything a module imports to read a data source and
send what it finds to the app as entities, edges, time series and events.

```
go get wayseer.dev/sdk
```

The app, its first-party modules and every external module build against this repository.

## Packages

| Package | What it holds |
|---|---|
| `wayseer.dev/sdk` | The module contract in one import: `Module`, `Sink`, `Register`, option decoding and secrets |
| `module` | The contract itself: the optional interfaces, actions, declared kinds and the registry |
| `model` | The world model: entities, edges, refs, kinds, values, status, events and places, and the store |
| `model/worldfile` | A recorded world, as `sdktest` replays it |
| `data` | Series, points, queries, filters, health, aggregation and the coalescer |
| `units` | Units of measure for series |
| `secret` | Secrets from the environment, files and the platform keyring |
| `manifest` | A module package's `manifest.yaml` |
| `sdktest` | The conformance suite and fakes for a module's own tests |
| `serve` | Serving a module from its own process (Tier 2) |
| `wire`, `proto/modulev1` | The Tier 2 wire format and its generated gRPC code |
| `wsmod` | Reading a signed module package |
| `netfail` | Classifying network failures for health messages |

`proto/mindseye/module/v1` is the Tier 2 contract. Its package name keeps Wayseer's old name,
so modules already built keep running.

## Writing a module

Wayseer's guide to writing modules walks through one from its first `Configure` to its
conformance test. A module imports only this SDK, the standard library and its own
dependencies, and its tests run `sdktest.Conform`.

## Working on the SDK

```
make check   # what CI runs: tests, vet and lint for every platform, the contract, a key scan
make proto   # regenerate proto/modulev1 after changing the contract
make help    # every target
```

Everything needs only Go. golangci-lint is pinned in `tools/go.mod`; buf and gitleaks run at
pinned versions through `go run`.

To try a change in the app or a module before tagging it, use a Go workspace beside them, for
example `go.work` in the module with `use . ../../sdk`. `go.work` is ignored by git; the app's
`scripts/workspace.sh` writes one for the whole of Wayseer Desktop.

## Versions

The SDK is at `v0`, so a minor version may break the Go API. From the first tag on,
`make check` fails on a change to the contract that breaks the wire since the last tag.

## Licence

MIT; see `LICENSE`.
