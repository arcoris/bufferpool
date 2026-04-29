// Package budget contains pure budget math for control-plane adapters.
//
// The helpers do not know about partitions, groups, pools, policies, or memory
// ownership. They operate on current values, optional limits, scores, and target
// bounds. A zero limit means unset or unbounded for usage/headroom/deficit
// helpers. Redistribution helpers are deterministic scaffolding and deliberately
// avoid complex water-filling until domain adapters need it.
//
// This package does not import the root bufferpool package and does not mutate
// runtime state. Root package adapters decide what each generic current, limit,
// score, and target means for PoolPartition and future PoolGroup control.
package budget
