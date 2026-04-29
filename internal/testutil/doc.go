// Package testutil contains shared helpers for repository tests.
//
// Helpers here are test-only infrastructure: panic assertions, controllable
// clocks, and other utilities that keep tests readable without leaking testing
// dependencies into production packages.
//
// testutil may depend on testing-oriented standard-library packages, but it
// must not become production runtime code. Production packages should not
// import testutil.
package testutil
