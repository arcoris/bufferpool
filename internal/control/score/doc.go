// Package score provides generic normalized score composition helpers.
//
// Scores explain control-plane signals through finite, clamped components.
// Components carry names, values, and weights so callers can report why a score
// was high without reverse-engineering the formula from a single float64. The
// current formulas are deterministic scaffolding, not final production tuning.
// Prepared scorers are intended for repeated controller evaluation because
// they normalize stable weights once. ScoreValue methods avoid diagnostic
// component allocation when a loop only needs scalar values, while
// NewWeightedScore and Score methods keep copied component explanations for
// diagnostics.
//
// Scores are not policy decisions. They must be adapted by the root package
// before they acquire partition or group meaning. This package does not import
// the root bufferpool package, does not mutate runtime state, and must not
// publish policy snapshots or execute trim.
package score
