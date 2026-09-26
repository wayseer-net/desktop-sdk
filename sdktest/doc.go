// Package sdktest tests modules: Conform runs the conformance suite every module must pass
// (lifecycle, snapshot then deltas, cancellation within 1 s, errors surfaced through Health,
// and the shape of query answers); Sink records what a module sends; Golden compares output
// with a file under testdata.
package sdktest
