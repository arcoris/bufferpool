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

import (
	"strings"

	"arcoris.dev/bufferpool/internal/multierr"
)

const (
	// errPoolConfigInvalidPolicyValidationMode is returned when PoolConfig
	// contains an unknown policy-validation mode.
	errPoolConfigInvalidPolicyValidationMode = "bufferpool.PoolConfig: unknown policy validation mode"

	// errPoolConfigInvalidCloseMode is returned when PoolConfig contains an
	// unknown close mode.
	errPoolConfigInvalidCloseMode = "bufferpool.PoolConfig: unknown close mode"

	// errPoolConfigInvalidClosedOperationMode is returned when PoolConfig
	// contains an unknown closed-operation mode.
	errPoolConfigInvalidClosedOperationMode = "bufferpool.PoolConfig: unknown closed-operation mode"

	// errPoolConfigInvalidPolicy is returned when the effective Pool policy does
	// not pass policy validation.
	errPoolConfigInvalidPolicy = "bufferpool.PoolConfig: invalid policy"
)

// PoolConfig defines construction-time configuration for one Pool.
//
// PoolConfig is the boundary between caller intent and runtime construction. It
// says which policy a Pool should use, how strictly that policy should be
// checked, and what lifecycle behavior the Pool should apply around close. It is
// not the runtime itself.
//
// PoolConfig is intentionally smaller than Policy. Policy describes static
// memory-retention behavior:
//
//   - retention limits;
//   - class-size profile;
//   - shard layout;
//   - admission behavior;
//   - pressure behavior;
//   - trim behavior;
//   - ownership behavior.
//
// PoolConfig describes how one concrete Pool should be constructed around the
// effective Policy:
//
//   - diagnostic name;
//   - policy source;
//   - whether policy validation is enforced;
//   - close-time retained-storage behavior;
//   - operation behavior after close.
//
// PoolConfig does not own runtime state. It does not contain class tables,
// class states, shards, buckets, counters, snapshots, current pressure,
// allocated memory, retained bytes, in-use bytes, lifecycle state, or controller
// state.
//
// The construction flow is intentionally split into three steps:
//
//	PoolConfig value
//	-> Normalize()
//	-> Validate()
//	-> future Pool construction
//
// Normalize fills unset fields with construction defaults and trims diagnostic
// names. Validate checks normalized modes and, when enabled, validates the
// effective Policy. Future Pool construction can then build class tables, class
// states, shard selectors, budgets, credits, lifecycle state, and close behavior
// from a config that has already been normalized.
//
// A zero PoolConfig is valid after normalization and resolves to DefaultPolicy
// and conservative lifecycle behavior.
type PoolConfig struct {
	// Name is a diagnostic pool name.
	//
	// The name is metadata only. It MUST NOT participate in class lookup,
	// admission decisions, budget distribution, ownership validation, or buffer
	// identity.
	//
	// Empty or whitespace-only names normalize to DefaultConfigPoolName.
	Name string

	// Policy is the effective runtime memory-retention policy for this Pool.
	//
	// A zero Policy normalizes to DefaultConfigPolicy(). Callers that want a
	// named preset should call the profile constructor themselves and pass the
	// resulting Policy:
	//
	//	policy := ThroughputPolicy()
	//	policy.Retention.HardRetainedBytes = 512 * MiB
	//
	//	config := PoolConfig{Policy: policy}
	//
	// PoolConfig intentionally does not contain a Profile field. This avoids
	// ambiguous construction states where both Profile and Policy are set and one
	// has to override the other.
	//
	// Normalize returns a policy value with caller-owned class-size storage. This
	// prevents later caller mutations to the original Policy slice from changing
	// a normalized PoolConfig value.
	Policy Policy

	// PolicyValidation controls whether the effective Policy is validated during
	// Pool construction.
	//
	// The unset value normalizes to the package construction default, which is
	// currently validation enabled. Disabling validation is useful for narrow
	// tests or benchmarks, but normal construction should keep validation enabled
	// so impossible class profiles, budgets, and pressure rules cannot reach the
	// runtime layer.
	PolicyValidation PoolPolicyValidationMode

	// CloseMode controls what Pool.Close should do with retained buffers after
	// the Pool has stopped accepting new operations.
	//
	// The unset value normalizes to the package construction default. The
	// default is to clear retained storage during close.
	//
	// Close-time clearing is lifecycle behavior, not ordinary trim policy. A
	// future Pool must first block new operations and only then call lower-level
	// clear helpers. Lower layers intentionally do not provide a class-wide
	// transactional barrier by themselves.
	CloseMode PoolCloseMode

	// ClosedOperations controls how operations attempted after close should be
	// handled.
	//
	// The unset value normalizes to the package construction default. The default
	// is strict rejection. Tolerant future APIs may choose to drop returned
	// buffers after close, but acquisition after close should still not recreate
	// runtime state.
	ClosedOperations PoolClosedOperationMode
}

// IsZero reports whether c contains no explicit construction settings.
//
// A zero PoolConfig is not an error. It means "use construction defaults".
func (c PoolConfig) IsZero() bool {
	return isPoolConfigNameUnset(c.Name) &&
		c.Policy.IsZero() &&
		c.PolicyValidation == PoolPolicyValidationModeUnset &&
		c.CloseMode == PoolCloseModeUnset &&
		c.ClosedOperations == PoolClosedOperationModeUnset
}

// DefaultPoolConfig returns the canonical construction config for a standalone
// Pool.
//
// The returned config owns its Policy value. Callers may modify the returned
// config and its policy without mutating package defaults or later calls to
// DefaultPoolConfig.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		Name:             DefaultConfigPoolName,
		Policy:           DefaultConfigPolicy(),
		PolicyValidation: defaultPoolPolicyValidationMode(),
		CloseMode:        defaultPoolCloseMode(),
		ClosedOperations: defaultPoolClosedOperationMode(),
	}
}

// Normalize returns a copy of c with construction defaults applied.
//
// Normalize does not validate the effective policy. Validation is performed by
// Validate so callers can inspect or adjust the normalized config before
// deciding whether to enforce validation.
func (c PoolConfig) Normalize() PoolConfig {
	normalized := c

	normalized.Name = normalizePoolConfigName(normalized.Name)
	normalized.Policy = effectivePoolConfigPolicy(normalized.Policy)
	normalized.PolicyValidation = effectivePoolPolicyValidationMode(normalized.PolicyValidation)
	normalized.CloseMode = effectivePoolCloseMode(normalized.CloseMode)
	normalized.ClosedOperations = effectivePoolClosedOperationMode(normalized.ClosedOperations)

	return normalized
}

// Validate validates Pool construction configuration.
//
// Validate first normalizes c and then checks the effective settings. This means
// a zero PoolConfig is valid if all construction defaults and the default Policy
// are valid.
//
// The returned error is classified as ErrInvalidOptions. If policy validation is
// enabled and the Policy itself is invalid, the returned error also unwraps the
// policy validation failure, so callers can still detect ErrInvalidPolicy with
// errors.Is.
func (c PoolConfig) Validate() error {
	normalized := c.Normalize()

	var err error

	if !isKnownPoolPolicyValidationMode(normalized.PolicyValidation) {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errPoolConfigInvalidPolicyValidationMode))
	}

	if !isKnownPoolCloseMode(normalized.CloseMode) {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errPoolConfigInvalidCloseMode))
	}

	if !isKnownPoolClosedOperationMode(normalized.ClosedOperations) {
		multierr.AppendInto(&err, newError(ErrInvalidOptions, errPoolConfigInvalidClosedOperationMode))
	}

	if normalized.PolicyValidation == PoolPolicyValidationModeEnabled {
		if policyErr := normalized.Policy.Validate(); policyErr != nil {
			multierr.AppendInto(&err, wrapError(ErrInvalidOptions, policyErr, errPoolConfigInvalidPolicy))
		}
	}

	return err
}

// EffectiveName returns the normalized diagnostic pool name.
//
// This helper applies only name normalization. It does not validate policy or
// allocate a normalized Policy value.
func (c PoolConfig) EffectiveName() string {
	return normalizePoolConfigName(c.Name)
}

// EffectivePolicy returns the normalized effective Policy.
//
// If c has no explicit Policy, this method returns DefaultConfigPolicy().
//
// The returned policy owns its class-size slice. Callers may mutate the returned
// policy as an override seed without changing c.Policy or later EffectivePolicy
// calls.
func (c PoolConfig) EffectivePolicy() Policy {
	return effectivePoolConfigPolicy(c.Policy)
}

// ShouldValidatePolicy reports whether Pool construction should validate the
// effective Policy.
func (c PoolConfig) ShouldValidatePolicy() bool {
	return effectivePoolPolicyValidationMode(c.PolicyValidation) == PoolPolicyValidationModeEnabled
}

// ShouldClearRetainedOnClose reports whether Pool.Close should clear retained
// storage after blocking new operations.
func (c PoolConfig) ShouldClearRetainedOnClose() bool {
	return effectivePoolCloseMode(c.CloseMode) == PoolCloseModeClearRetained
}

// ShouldKeepRetainedOnClose reports whether Pool.Close should leave retained
// storage untouched.
//
// This mode is primarily useful for tests or diagnostic experiments. Normal
// production close should clear retained buffers after lifecycle shutdown.
func (c PoolConfig) ShouldKeepRetainedOnClose() bool {
	return effectivePoolCloseMode(c.CloseMode) == PoolCloseModeKeepRetained
}

// ShouldRejectOperationsAfterClose reports whether operations after close should
// be rejected.
func (c PoolConfig) ShouldRejectOperationsAfterClose() bool {
	return effectivePoolClosedOperationMode(c.ClosedOperations) == PoolClosedOperationModeReject
}

// ShouldDropReturnedBuffersAfterClose reports whether return-path operations
// after close should be treated as no-retain drops.
//
// Acquisition after close should still avoid creating or reviving runtime state.
func (c PoolConfig) ShouldDropReturnedBuffersAfterClose() bool {
	return effectivePoolClosedOperationMode(c.ClosedOperations) == PoolClosedOperationModeDropReturns
}

// PoolPolicyValidationMode controls whether Pool construction validates the
// effective Policy.
//
// Policy validation is a construction-time safety check. It protects lower
// runtime components from impossible class tables, contradictory budgets,
// invalid pressure behavior, and ownership settings that cannot be enforced.
//
// The mode is separate from Policy because the same Policy value may be used in
// different contexts: ordinary construction should validate it, while some
// low-level tests may intentionally build invalid values to exercise failure
// paths.
type PoolPolicyValidationMode uint8

const (
	// PoolPolicyValidationModeUnset means construction should use the package
	// default policy-validation behavior.
	PoolPolicyValidationModeUnset PoolPolicyValidationMode = iota

	// PoolPolicyValidationModeEnabled validates the effective Policy during Pool
	// construction.
	PoolPolicyValidationModeEnabled

	// PoolPolicyValidationModeDisabled skips policy validation during Pool
	// construction.
	//
	// This mode is intended for tests, benchmarks, or advanced internal use.
	// Production code should normally keep policy validation enabled because
	// invalid policy values can violate lower runtime invariants.
	PoolPolicyValidationModeDisabled
)

// String returns a stable diagnostic label for m.
func (m PoolPolicyValidationMode) String() string {
	switch m {
	case PoolPolicyValidationModeUnset:
		return "unset"
	case PoolPolicyValidationModeEnabled:
		return "enabled"
	case PoolPolicyValidationModeDisabled:
		return "disabled"
	default:
		return "unknown"
	}
}

// PoolCloseMode controls retained-storage behavior during Pool.Close.
//
// Close behavior belongs to owner lifecycle, not to the raw static runtime.
// classState.clear and shard.clear remove storage that they can see at the time
// they run, but they are not global lifecycle barriers. A Pool that wants strong
// cleanup must first stop new operations and then apply the selected close mode.
type PoolCloseMode uint8

const (
	// PoolCloseModeUnset means construction should use the package default close
	// behavior.
	PoolCloseModeUnset PoolCloseMode = iota

	// PoolCloseModeClearRetained clears retained buffers during close after the
	// Pool has stopped accepting new operations.
	//
	// This is the production-oriented default. Lower-level classState.clear is
	// intentionally non-transactional; Pool lifecycle must first block new
	// operations and only then clear retained storage.
	PoolCloseModeClearRetained

	// PoolCloseModeKeepRetained leaves retained buffers untouched during close.
	//
	// This mode is not the normal production posture. It is useful for tests,
	// diagnostics, or specialized owner code that wants to control cleanup
	// separately.
	PoolCloseModeKeepRetained
)

// String returns a stable diagnostic label for m.
func (m PoolCloseMode) String() string {
	switch m {
	case PoolCloseModeUnset:
		return "unset"
	case PoolCloseModeClearRetained:
		return "clear_retained"
	case PoolCloseModeKeepRetained:
		return "keep_retained"
	default:
		return "unknown"
	}
}

// PoolClosedOperationMode controls operation behavior after Pool close.
//
// The mode defines the public API posture after lifecycle shutdown. It does not
// reopen storage, recreate class state, or make lower runtime objects usable
// after their owner has closed.
type PoolClosedOperationMode uint8

const (
	// PoolClosedOperationModeUnset means construction should use the package
	// default closed-operation behavior.
	PoolClosedOperationModeUnset PoolClosedOperationMode = iota

	// PoolClosedOperationModeReject rejects operations attempted after close.
	//
	// Public APIs that can return errors should report ErrClosed. APIs that
	// cannot return errors should still avoid mutating retained storage or
	// reviving runtime state.
	PoolClosedOperationModeReject

	// PoolClosedOperationModeDropReturns drops returned buffers after close.
	//
	// This mode is tolerant for return paths. It can be useful when callers use
	// deferred returns and do not want a late Put-like operation to fail after
	// the owner has already closed. Acquisition after close should still reject.
	PoolClosedOperationModeDropReturns
)

// String returns a stable diagnostic label for m.
func (m PoolClosedOperationMode) String() string {
	switch m {
	case PoolClosedOperationModeUnset:
		return "unset"
	case PoolClosedOperationModeReject:
		return "reject"
	case PoolClosedOperationModeDropReturns:
		return "drop_returns"
	default:
		return "unknown"
	}
}

// isPoolConfigNameUnset reports whether name should be treated as unspecified.
//
// Pool names are diagnostic metadata. Whitespace-only input is normalized the
// same way as an empty name so construction does not preserve accidental
// formatting as a meaningful runtime identifier.
func isPoolConfigNameUnset(name string) bool {
	return strings.TrimSpace(name) == ""
}

// normalizePoolConfigName returns the effective diagnostic Pool name.
//
// The helper is intentionally narrow: it does not validate Policy and does not
// allocate or clone any runtime structures. It only applies the default name
// when caller input is empty after trimming.
func normalizePoolConfigName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultConfigPoolName
	}

	return name
}

// effectivePoolConfigPolicy returns the construction Policy after defaults.
//
// A zero Policy means "use the package default policy". An explicit non-zero
// Policy is copied before being returned so later caller mutations to
// ClassPolicy.Sizes cannot change the normalized config value.
func effectivePoolConfigPolicy(policy Policy) Policy {
	if policy.IsZero() {
		return DefaultConfigPolicy()
	}

	return copyPoolConfigPolicy(policy)
}

// copyPoolConfigPolicy returns policy with caller-owned mutable slice fields.
//
// Policy is mostly value-shaped, but ClassPolicy.Sizes is a slice. PoolConfig
// helpers clone that slice at every construction boundary to avoid sharing
// mutable class profiles between caller values and runtime values.
func copyPoolConfigPolicy(policy Policy) Policy {
	policy.Classes.Sizes = policy.Classes.SizesCopy()
	return policy
}

// effectivePoolPolicyValidationMode resolves unset validation mode to the
// package construction default.
//
// Unknown explicit modes are deliberately preserved so Validate can report a
// stable ErrInvalidOptions failure instead of silently replacing bad input.
func effectivePoolPolicyValidationMode(mode PoolPolicyValidationMode) PoolPolicyValidationMode {
	if mode == PoolPolicyValidationModeUnset {
		return defaultPoolPolicyValidationMode()
	}

	return mode
}

// effectivePoolCloseMode resolves unset close behavior to the package default.
//
// Unknown explicit modes are preserved for Validate. This keeps normalization
// side-effect free and makes validation responsible for rejecting bad enum
// values.
func effectivePoolCloseMode(mode PoolCloseMode) PoolCloseMode {
	if mode == PoolCloseModeUnset {
		return defaultPoolCloseMode()
	}

	return mode
}

// effectivePoolClosedOperationMode resolves unset post-close behavior to the
// package default.
//
// Unknown explicit modes are preserved so caller mistakes are visible during
// validation instead of being silently normalized away.
func effectivePoolClosedOperationMode(mode PoolClosedOperationMode) PoolClosedOperationMode {
	if mode == PoolClosedOperationModeUnset {
		return defaultPoolClosedOperationMode()
	}

	return mode
}

// defaultPoolPolicyValidationMode adapts the package boolean default into the
// PoolConfig enum.
//
// Keeping this adapter here isolates Pool construction from the representation
// chosen by config_defaults.go.
func defaultPoolPolicyValidationMode() PoolPolicyValidationMode {
	if DefaultConfigPolicyValidation() {
		return PoolPolicyValidationModeEnabled
	}

	return PoolPolicyValidationModeDisabled
}

// defaultPoolCloseMode adapts close-time retained-storage defaults into the
// PoolConfig enum.
//
// The default expresses owner lifecycle behavior. It does not alter lower-level
// classState or shard clear semantics.
func defaultPoolCloseMode() PoolCloseMode {
	if DefaultConfigCloseTrimsRetained() {
		return PoolCloseModeClearRetained
	}

	return PoolCloseModeKeepRetained
}

// defaultPoolClosedOperationMode adapts post-close operation defaults into the
// PoolConfig enum.
//
// This keeps direct Pool construction aligned with package defaults while still
// allowing tests and specialized internal callers to choose drop-return mode.
func defaultPoolClosedOperationMode() PoolClosedOperationMode {
	if DefaultConfigCloseRejectsOperations() {
		return PoolClosedOperationModeReject
	}

	return PoolClosedOperationModeDropReturns
}

// isKnownPoolPolicyValidationMode reports whether mode is one of the enum
// values PoolConfig can execute.
//
// The unset value is considered known because Normalize resolves it before Pool
// construction and Validate accepts zero configs.
func isKnownPoolPolicyValidationMode(mode PoolPolicyValidationMode) bool {
	switch mode {
	case PoolPolicyValidationModeUnset,
		PoolPolicyValidationModeEnabled,
		PoolPolicyValidationModeDisabled:
		return true
	default:
		return false
	}
}

// isKnownPoolCloseMode reports whether mode is one of the supported close
// behaviors.
//
// Validation uses this helper after normalization, but accepting unset keeps the
// helper safe for direct tests and narrow callers.
func isKnownPoolCloseMode(mode PoolCloseMode) bool {
	switch mode {
	case PoolCloseModeUnset,
		PoolCloseModeClearRetained,
		PoolCloseModeKeepRetained:
		return true
	default:
		return false
	}
}

// isKnownPoolClosedOperationMode reports whether mode is one of the supported
// post-close operation behaviors.
//
// The helper does not decide whether a particular operation should be admitted;
// it only validates the enum domain used by lifecycle admission.
func isKnownPoolClosedOperationMode(mode PoolClosedOperationMode) bool {
	switch mode {
	case PoolClosedOperationModeUnset,
		PoolClosedOperationModeReject,
		PoolClosedOperationModeDropReturns:
		return true
	default:
		return false
	}
}
