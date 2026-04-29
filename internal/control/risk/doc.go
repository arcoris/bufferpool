// Package risk scores ownership and return-failure safety signals.
//
// Risk values explain misuse and safety pressure. Ownership violations and
// double releases are intentionally high severity, while closed-resource handoff
// failures stay separable from admission/runtime failures. That separation lets
// root adapters distinguish expected hard-close diagnostics from unexpected
// ownership or return-path problems.
//
// The package does not directly mutate policies, publish runtime snapshots, or
// import the root bufferpool package. It is control-plane only.
package risk
