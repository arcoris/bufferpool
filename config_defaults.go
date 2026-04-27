/*
  Copyright 2026 The ARCORIS Authors

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

package bufferpool

import "time"

const (
	// DefaultConfigPartitionCount is the default number of control-plane
	// partitions used by group-level configuration when the caller does not
	// specify partitioning explicitly.
	//
	// A single partition keeps the default construction path simple and
	// predictable. It still preserves the mature architecture:
	//
	//	PoolGroup
	//	-> PoolPartition
	//	-> Pool
	//	-> classState
	//	-> shard
	//	-> bucket
	//
	// Larger deployments may raise this value later through GroupConfig when
	// many pools require bounded controller scans, partition-local ownership, or
	// reduced control-plane fan-out.
	DefaultConfigPartitionCount = 1

	// DefaultConfigControllerEnabled controls whether owner configuration should
	// start background control loops by default.
	//
	// The default is false because the initial public runtime should be safe and
	// deterministic without hidden goroutines. A future PoolGroup or
	// PoolPartition config may enable controller loops explicitly once adaptive
	// budget redistribution, workload windows, pressure handling, and bounded
	// trim execution exist.
	DefaultConfigControllerEnabled = false

	// DefaultConfigValidatePolicy controls whether construction config should
	// validate the effective Policy before building runtime state.
	//
	// The default is true. Policy validation is cheap compared with runtime
	// construction and prevents invalid class profiles, impossible budgets,
	// invalid pressure behavior, and contradictory ownership settings from
	// reaching lower runtime components.
	DefaultConfigValidatePolicy = true

	// DefaultConfigTrimOnClose controls whether close paths should physically
	// remove retained buffers after the owner has stopped new operations.
	//
	// This is a construction/lifecycle default, not a trim-policy target. The
	// trim policy controls ordinary background/corrective trim behavior. Close
	// behavior belongs to owner lifecycle because strong cleanup requires first
	// blocking new get/put operations and only then clearing retained storage.
	DefaultConfigTrimOnClose = true

	// DefaultConfigRejectOperationsAfterClose controls the default behavior for
	// operations attempted after a runtime owner has closed.
	//
	// The default is true because operations after close should not silently
	// mutate counters, retain buffers, or restart lifecycle-dependent state.
	// Future tolerant APIs may choose to drop returned buffers after close, but
	// construction defaults should favor explicit lifecycle correctness.
	DefaultConfigRejectOperationsAfterClose = true
)

const (
	// DefaultConfigControllerInterval is the default cadence for future
	// partition-controller cycles.
	//
	// Controller cycles are cold-path work: snapshot harvesting, window delta
	// computation, budget publication, pressure observation, and trim scheduling.
	// The value is intentionally separate from TrimPolicy.Interval. Trim cadence
	// is policy behavior; controller cadence is owner runtime scheduling.
	DefaultConfigControllerInterval time.Duration = time.Second

	// DefaultConfigControllerShutdownTimeout is the default maximum time a future
	// owner should wait for controller shutdown during close.
	//
	// The value is intentionally conservative. Shutdown should not block
	// indefinitely if a background controller is enabled in a future group or
	// partition config.
	DefaultConfigControllerShutdownTimeout time.Duration = 5 * time.Second
)

const (
	// DefaultConfigPoolName is the default diagnostic name for a pool when a
	// future PoolConfig does not provide one.
	//
	// Names are diagnostic metadata only. They must not participate in buffer
	// admission, class lookup, budget distribution, or ownership validation.
	DefaultConfigPoolName = "default"

	// DefaultConfigGroupName is the default diagnostic name for a group when a
	// future GroupConfig does not provide one.
	//
	// Names are intentionally separate from lifecycle identity. A runtime owner
	// should not rely on a name for uniqueness unless a future registry explicitly
	// enforces it.
	DefaultConfigGroupName = "default"

	// DefaultConfigPartitionNamePrefix is the default prefix used when future
	// group construction creates unnamed partitions.
	//
	// A partition name is diagnostic metadata. Partition identity should still be
	// represented by stable indexes or IDs owned by the group/partition registry.
	DefaultConfigPartitionNamePrefix = "partition"
)

// DefaultConfigPolicy returns the policy used by configuration normalization
// when no explicit policy is supplied.
//
// This function intentionally delegates to DefaultPolicy instead of duplicating
// policy fields. policy_defaults.go remains the single source of truth for the
// canonical default runtime behavior.
func DefaultConfigPolicy() Policy {
	return DefaultPolicy()
}

// DefaultConfigProfile returns the named profile used by configuration parsing
// when a profile-oriented config surface needs an explicit default profile.
//
// The profile default is intentionally represented as PolicyProfileDefault
// rather than as a separate policy copy. Config parsing can keep the user's
// intent visible:
//
//	profile unset       -> use DefaultConfigProfile()
//	profile throughput  -> use ThroughputPolicy()
//	custom policy       -> use the supplied Policy directly
//
// Go callers should usually use DefaultConfigPolicy or a concrete profile
// constructor instead of routing through profile names.
func DefaultConfigProfile() PolicyProfile {
	return PolicyProfileDefault
}

// DefaultConfigControllerPolicy returns whether future owner configs should
// enable a controller loop when no explicit setting is provided.
//
// A function is used instead of requiring callers to depend directly on the
// constant. This leaves room for future owner-specific normalization without
// changing the public construction helpers.
func DefaultConfigControllerPolicy() bool {
	return DefaultConfigControllerEnabled
}

// DefaultConfigPolicyValidation returns whether future construction config
// should validate the effective policy by default.
//
// This helper exists so PoolConfig, GroupConfig, and PartitionConfig
// normalization can share the same default without duplicating policy-validation
// assumptions.
func DefaultConfigPolicyValidation() bool {
	return DefaultConfigValidatePolicy
}

// DefaultConfigCloseTrimsRetained returns whether close normalization should
// clear retained storage after blocking new operations.
//
// Close-time clear is lifecycle behavior. It must happen only after the owner
// prevents new get/put operations. Lower-level classState.clear remains
// intentionally non-transactional and relies on owner lifecycle for strong
// shutdown semantics.
func DefaultConfigCloseTrimsRetained() bool {
	return DefaultConfigTrimOnClose
}

// DefaultConfigCloseRejectsOperations returns whether operations after close
// should be rejected by default.
//
// Public APIs may expose both strict and tolerant variants later. The default
// construction posture should remain strict because post-close operations are
// usually lifecycle bugs.
func DefaultConfigCloseRejectsOperations() bool {
	return DefaultConfigRejectOperationsAfterClose
}

// DefaultConfigControllerCadence returns the default controller cycle cadence.
//
// The value is separate from trim cadence. A controller cycle may publish
// budgets, observe pressure, compute workload windows, and schedule trim work;
// trim policy decides how much physical removal is allowed.
func DefaultConfigControllerCadence() time.Duration {
	return DefaultConfigControllerInterval
}

// DefaultConfigControllerStopTimeout returns the default controller shutdown
// timeout for future owner configs.
func DefaultConfigControllerStopTimeout() time.Duration {
	return DefaultConfigControllerShutdownTimeout
}
