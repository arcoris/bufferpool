// Package pressure provides generic pressure levels and threshold helpers.
//
// Thresholds are optional per level: zero means disabled for that level.
// Classification checks critical, then high, then medium so the highest
// configured exceeded threshold wins. Validation rejects contradictory
// configured thresholds but accepts partial chains such as medium+critical
// without high.
//
// Root package adapters map these generic levels to domain pressure types. The
// Classifier type validates stable thresholds once for repeated evaluation. A
// zero Classifier is equivalent to no configured pressure thresholds and
// classifies every value as normal. The package does not sample runtime
// metrics, mutate policies, publish runtime snapshots, or import the root
// bufferpool package.
// Repository-level control boundaries and verification gates are documented in
// docs/design/control-algorithms.md.
package pressure
