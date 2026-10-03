// Package data holds the data plane: query shapes (series, filters), the series cache,
// downsampling, and the coalescer that turns module deltas into world-model commits with
// per-module freshness. It is pure; modules and lenses both depend on it.
package data
