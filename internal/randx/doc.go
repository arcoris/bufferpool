// Package randx contains deterministic low-level randomization helpers.
//
// The package supports internal shard selection and hash mixing without pulling
// policy or runtime ownership concerns into the randomization layer. Helpers
// are intentionally small and allocation-conscious because they may be used near
// hot paths.
//
// randx must not import the root bufferpool package. It should expose only
// generic random or mixing primitives, leaving Pool and Partition code to decide
// how those values map to shards or candidates.
package randx
