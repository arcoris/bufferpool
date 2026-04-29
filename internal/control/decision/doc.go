// Package decision contains generic recommendation value types.
//
// Decisions are pure evaluations. Recommendations and actions contain a kind,
// confidence, and reason, but they do not execute work. None and observe are
// non-actionable; grow, shrink, trim, and investigate are actionable only after
// a root adapter maps them to a domain-specific operation.
//
// No policy mutation happens here. This package does not import the root
// bufferpool package and should not be used for hot-path state changes.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package decision
