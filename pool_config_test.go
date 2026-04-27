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
	"errors"
	"reflect"
	"testing"

	"arcoris.dev/bufferpool/internal/multierr"
)

// TestPoolConfigIsZero verifies the zero-state contract for PoolConfig.
//
// A zero PoolConfig is not invalid. It means "use construction defaults".
// However, IsZero must still distinguish a completely unset config from one that
// explicitly sets a diagnostic name, policy, validation mode, close mode, or
// closed-operation behavior.
func TestPoolConfigIsZero(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config PoolConfig
		want   bool
	}{
		{
			name:   "zero",
			config: PoolConfig{},
			want:   true,
		},
		{
			name: "whitespace name is still zero",
			config: PoolConfig{
				Name: " \t\n ",
			},
			want: true,
		},
		{
			name: "name",
			config: PoolConfig{
				Name: "http-body",
			},
			want: false,
		},
		{
			name: "policy",
			config: PoolConfig{
				Policy: DefaultPolicy(),
			},
			want: false,
		},
		{
			name: "policy validation mode",
			config: PoolConfig{
				PolicyValidation: PoolPolicyValidationModeEnabled,
			},
			want: false,
		},
		{
			name: "close mode",
			config: PoolConfig{
				CloseMode: PoolCloseModeClearRetained,
			},
			want: false,
		},
		{
			name: "closed operation mode",
			config: PoolConfig{
				ClosedOperations: PoolClosedOperationModeReject,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.config.IsZero(); got != tt.want {
				t.Fatalf("PoolConfig.IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDefaultPoolConfig verifies the canonical standalone Pool construction
// defaults.
//
// DefaultPoolConfig should be a fully normalized config. It must not rely on
// Pool construction to fill mandatory construction behavior.
func TestDefaultPoolConfig(t *testing.T) {
	t.Parallel()

	config := DefaultPoolConfig()

	if config.IsZero() {
		t.Fatal("DefaultPoolConfig() returned zero config")
	}

	if config.Name != DefaultConfigPoolName {
		t.Fatalf("DefaultPoolConfig().Name = %q, want %q", config.Name, DefaultConfigPoolName)
	}

	if !reflect.DeepEqual(config.Policy, DefaultConfigPolicy()) {
		t.Fatal("DefaultPoolConfig().Policy does not match DefaultConfigPolicy()")
	}

	if config.PolicyValidation != PoolPolicyValidationModeEnabled {
		t.Fatalf("DefaultPoolConfig().PolicyValidation = %s, want enabled", config.PolicyValidation)
	}

	if config.CloseMode != PoolCloseModeClearRetained {
		t.Fatalf("DefaultPoolConfig().CloseMode = %s, want clear_retained", config.CloseMode)
	}

	if config.ClosedOperations != PoolClosedOperationModeReject {
		t.Fatalf("DefaultPoolConfig().ClosedOperations = %s, want reject", config.ClosedOperations)
	}

	if err := config.Validate(); err != nil {
		t.Fatalf("DefaultPoolConfig().Validate() returned error: %v", err)
	}
}

// TestDefaultPoolConfigReturnsIndependentPolicy verifies that construction
// defaults do not expose shared mutable class-size slices.
//
// Users are expected to take default configs or profile-derived configs and
// modify them. Mutating one returned config must not corrupt later defaults.
func TestDefaultPoolConfigReturnsIndependentPolicy(t *testing.T) {
	t.Parallel()

	first := DefaultPoolConfig()
	second := DefaultPoolConfig()

	if len(first.Policy.Classes.Sizes) == 0 {
		t.Fatal("DefaultPoolConfig() returned policy with no class sizes")
	}

	originalSecondFirstClass := second.Policy.Classes.Sizes[0]

	first.Policy.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)
	first.Policy.Retention.HardRetainedBytes = 1

	third := DefaultPoolConfig()

	if second.Policy.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first default config changed second config class size: got %s, want %s",
			second.Policy.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}

	if third.Policy.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first default config changed later default config class size: got %s, want %s",
			third.Policy.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}
}

// TestPoolConfigNormalize verifies construction-default normalization.
//
// Normalize should trim diagnostic names, fill missing policies, and resolve
// unset construction modes. It must preserve explicit user choices while still
// returning caller-owned policy slice storage.
func TestPoolConfigNormalize(t *testing.T) {
	t.Parallel()

	t.Run("zero config", func(t *testing.T) {
		t.Parallel()

		normalized := (PoolConfig{}).Normalize()

		if normalized.Name != DefaultConfigPoolName {
			t.Fatalf("Normalize().Name = %q, want %q", normalized.Name, DefaultConfigPoolName)
		}

		if !reflect.DeepEqual(normalized.Policy, DefaultConfigPolicy()) {
			t.Fatal("Normalize().Policy does not match DefaultConfigPolicy()")
		}

		if normalized.PolicyValidation != PoolPolicyValidationModeEnabled {
			t.Fatalf("Normalize().PolicyValidation = %s, want enabled", normalized.PolicyValidation)
		}

		if normalized.CloseMode != PoolCloseModeClearRetained {
			t.Fatalf("Normalize().CloseMode = %s, want clear_retained", normalized.CloseMode)
		}

		if normalized.ClosedOperations != PoolClosedOperationModeReject {
			t.Fatalf("Normalize().ClosedOperations = %s, want reject", normalized.ClosedOperations)
		}
	})

	t.Run("explicit values are preserved", func(t *testing.T) {
		t.Parallel()

		policy := ThroughputPolicy()

		config := PoolConfig{
			Name:             "  compression  ",
			Policy:           policy,
			PolicyValidation: PoolPolicyValidationModeDisabled,
			CloseMode:        PoolCloseModeKeepRetained,
			ClosedOperations: PoolClosedOperationModeDropReturns,
		}

		normalized := config.Normalize()

		if normalized.Name != "compression" {
			t.Fatalf("Normalize().Name = %q, want %q", normalized.Name, "compression")
		}

		if !reflect.DeepEqual(normalized.Policy, policy) {
			t.Fatal("Normalize() did not preserve explicit policy")
		}

		if normalized.PolicyValidation != PoolPolicyValidationModeDisabled {
			t.Fatalf("Normalize().PolicyValidation = %s, want disabled", normalized.PolicyValidation)
		}

		if normalized.CloseMode != PoolCloseModeKeepRetained {
			t.Fatalf("Normalize().CloseMode = %s, want keep_retained", normalized.CloseMode)
		}

		if normalized.ClosedOperations != PoolClosedOperationModeDropReturns {
			t.Fatalf("Normalize().ClosedOperations = %s, want drop_returns", normalized.ClosedOperations)
		}

		if len(policy.Classes.Sizes) == 0 {
			t.Fatal("test policy has no class sizes")
		}

		originalNormalizedFirstClass := normalized.Policy.Classes.Sizes[0]
		policy.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

		if normalized.Policy.Classes.Sizes[0] != originalNormalizedFirstClass {
			t.Fatalf("Normalize() reused explicit policy class-size backing storage: got %s, want %s",
				normalized.Policy.Classes.Sizes[0],
				originalNormalizedFirstClass,
			)
		}
	})

	t.Run("whitespace name uses default", func(t *testing.T) {
		t.Parallel()

		normalized := PoolConfig{Name: " \t\n "}.Normalize()

		if normalized.Name != DefaultConfigPoolName {
			t.Fatalf("Normalize().Name = %q, want %q", normalized.Name, DefaultConfigPoolName)
		}
	})
}

// TestPoolConfigNormalizeReturnsIndependentDefaultPolicy verifies that
// Normalize does not expose shared mutable default policy storage.
//
// Pool construction may normalize a zero config, tune the resulting policy, and
// then build runtime state. That tuning must not affect later normalization.
func TestPoolConfigNormalizeReturnsIndependentDefaultPolicy(t *testing.T) {
	t.Parallel()

	first := (PoolConfig{}).Normalize()
	second := (PoolConfig{}).Normalize()

	if len(first.Policy.Classes.Sizes) == 0 {
		t.Fatal("Normalize() returned policy with no class sizes")
	}

	originalSecondFirstClass := second.Policy.Classes.Sizes[0]
	first.Policy.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

	third := (PoolConfig{}).Normalize()

	if second.Policy.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first normalized default changed second normalized default: got %s, want %s",
			second.Policy.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}

	if third.Policy.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first normalized default changed later normalized default: got %s, want %s",
			third.Policy.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}
}

// TestPoolConfigValidate verifies ordinary validation outcomes.
//
// PoolConfig validation is construction validation. Invalid construction modes
// are ErrInvalidOptions. If policy validation is enabled and the effective
// policy is invalid, the returned error must also expose ErrInvalidPolicy through
// the wrapped cause.
func TestPoolConfigValidate(t *testing.T) {
	t.Parallel()

	t.Run("zero config is valid after normalization", func(t *testing.T) {
		t.Parallel()

		if err := (PoolConfig{}).Validate(); err != nil {
			t.Fatalf("zero PoolConfig.Validate() returned error: %v", err)
		}
	})

	t.Run("explicit valid profile policy is valid", func(t *testing.T) {
		t.Parallel()

		config := PoolConfig{
			Name:   "throughput",
			Policy: ThroughputPolicy(),
		}

		if err := config.Validate(); err != nil {
			t.Fatalf("PoolConfig.Validate() returned error for valid throughput policy: %v", err)
		}
	})

	t.Run("invalid policy is reported when validation enabled", func(t *testing.T) {
		t.Parallel()

		config := PoolConfig{
			Policy: invalidPoolConfigPolicy(),
		}

		err := config.Validate()
		if err == nil {
			t.Fatal("PoolConfig.Validate() returned nil for invalid policy")
		}

		if !errors.Is(err, ErrInvalidOptions) {
			t.Fatalf("PoolConfig.Validate() error does not match ErrInvalidOptions: %v", err)
		}

		if !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("PoolConfig.Validate() error does not unwrap ErrInvalidPolicy: %v", err)
		}
	})

	t.Run("invalid policy is skipped when validation disabled", func(t *testing.T) {
		t.Parallel()

		config := PoolConfig{
			Policy:           invalidPoolConfigPolicy(),
			PolicyValidation: PoolPolicyValidationModeDisabled,
		}

		if err := config.Validate(); err != nil {
			t.Fatalf("PoolConfig.Validate() returned error with policy validation disabled: %v", err)
		}
	})
}

// TestPoolConfigValidateInvalidModes verifies invalid enum values.
//
// Unknown construction modes must be rejected as invalid options. This prevents
// externally decoded numeric values from silently becoming meaningful runtime
// behavior.
func TestPoolConfigValidateInvalidModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    PoolConfig
		wantCount int
	}{
		{
			name: "invalid policy validation mode",
			config: PoolConfig{
				PolicyValidation: PoolPolicyValidationMode(255),
			},
			wantCount: 1,
		},
		{
			name: "invalid close mode",
			config: PoolConfig{
				CloseMode: PoolCloseMode(255),
			},
			wantCount: 1,
		},
		{
			name: "invalid closed operation mode",
			config: PoolConfig{
				ClosedOperations: PoolClosedOperationMode(255),
			},
			wantCount: 1,
		},
		{
			name: "all invalid modes",
			config: PoolConfig{
				PolicyValidation: PoolPolicyValidationMode(255),
				CloseMode:        PoolCloseMode(254),
				ClosedOperations: PoolClosedOperationMode(253),
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.config.Validate()
			if err == nil {
				t.Fatal("PoolConfig.Validate() returned nil for invalid mode")
			}

			if !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("PoolConfig.Validate() error does not match ErrInvalidOptions: %v", err)
			}

			if got := len(multierr.Errors(err)); got != tt.wantCount {
				t.Fatalf("PoolConfig.Validate() error count = %d, want %d: %v", got, tt.wantCount, err)
			}
		})
	}
}

// TestPoolConfigEffectiveHelpers verifies convenience helpers over normalized
// configuration.
//
// These helpers are intended for future Pool construction code so it can read
// normalized behavior without duplicating normalization logic at every call site.
func TestPoolConfigEffectiveHelpers(t *testing.T) {
	t.Parallel()

	t.Run("default helpers", func(t *testing.T) {
		t.Parallel()

		config := PoolConfig{}

		if got := config.EffectiveName(); got != DefaultConfigPoolName {
			t.Fatalf("EffectiveName() = %q, want %q", got, DefaultConfigPoolName)
		}

		if !reflect.DeepEqual(config.EffectivePolicy(), DefaultConfigPolicy()) {
			t.Fatal("EffectivePolicy() does not match DefaultConfigPolicy()")
		}

		if !config.ShouldValidatePolicy() {
			t.Fatal("ShouldValidatePolicy() = false, want true")
		}

		if !config.ShouldClearRetainedOnClose() {
			t.Fatal("ShouldClearRetainedOnClose() = false, want true")
		}

		if config.ShouldKeepRetainedOnClose() {
			t.Fatal("ShouldKeepRetainedOnClose() = true, want false")
		}

		if !config.ShouldRejectOperationsAfterClose() {
			t.Fatal("ShouldRejectOperationsAfterClose() = false, want true")
		}

		if config.ShouldDropReturnedBuffersAfterClose() {
			t.Fatal("ShouldDropReturnedBuffersAfterClose() = true, want false")
		}
	})

	t.Run("explicit helpers", func(t *testing.T) {
		t.Parallel()

		config := PoolConfig{
			Name:             "  logs  ",
			Policy:           MemoryConstrainedPolicy(),
			PolicyValidation: PoolPolicyValidationModeDisabled,
			CloseMode:        PoolCloseModeKeepRetained,
			ClosedOperations: PoolClosedOperationModeDropReturns,
		}

		if got := config.EffectiveName(); got != "logs" {
			t.Fatalf("EffectiveName() = %q, want %q", got, "logs")
		}

		if !reflect.DeepEqual(config.EffectivePolicy(), MemoryConstrainedPolicy()) {
			t.Fatal("EffectivePolicy() does not match explicit policy")
		}

		if config.ShouldValidatePolicy() {
			t.Fatal("ShouldValidatePolicy() = true, want false")
		}

		if config.ShouldClearRetainedOnClose() {
			t.Fatal("ShouldClearRetainedOnClose() = true, want false")
		}

		if !config.ShouldKeepRetainedOnClose() {
			t.Fatal("ShouldKeepRetainedOnClose() = false, want true")
		}

		if config.ShouldRejectOperationsAfterClose() {
			t.Fatal("ShouldRejectOperationsAfterClose() = true, want false")
		}

		if !config.ShouldDropReturnedBuffersAfterClose() {
			t.Fatal("ShouldDropReturnedBuffersAfterClose() = false, want true")
		}
	})
}

// TestPoolConfigEffectivePolicyReturnsIndependentDefault verifies that
// EffectivePolicy does not expose shared mutable default class-size storage.
//
// This protects the expected user workflow:
//
//	policy := PoolConfig{}.EffectivePolicy()
//	policy.Classes.Sizes[0] = customSize
//
// Such mutation must not affect later default policy construction.
func TestPoolConfigEffectivePolicyReturnsIndependentDefault(t *testing.T) {
	t.Parallel()

	first := (PoolConfig{}).EffectivePolicy()
	second := (PoolConfig{}).EffectivePolicy()

	if len(first.Classes.Sizes) == 0 {
		t.Fatal("EffectivePolicy() returned policy with no class sizes")
	}

	originalSecondFirstClass := second.Classes.Sizes[0]

	first.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

	third := (PoolConfig{}).EffectivePolicy()

	if second.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first effective policy changed second policy: got %s, want %s",
			second.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}

	if third.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first effective policy changed later policy: got %s, want %s",
			third.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}
}

// TestPoolConfigEffectivePolicyCopiesExplicitPolicy verifies that EffectivePolicy
// returns an isolated policy even when the caller supplied an explicit Policy.
//
// This preserves the override workflow for future Pool construction code:
// callers can resolve the effective policy, mutate it locally, and know that the
// original config remains unchanged.
func TestPoolConfigEffectivePolicyCopiesExplicitPolicy(t *testing.T) {
	t.Parallel()

	policy := SecurePolicy()
	config := PoolConfig{
		Policy: policy,
	}

	first := config.EffectivePolicy()
	second := config.EffectivePolicy()

	if len(policy.Classes.Sizes) == 0 || len(first.Classes.Sizes) == 0 || len(second.Classes.Sizes) == 0 {
		t.Fatal("test policy has no class sizes")
	}

	originalConfigFirstClass := config.Policy.Classes.Sizes[0]
	originalSecondFirstClass := second.Classes.Sizes[0]

	first.Classes.Sizes[0] = ClassSizeFromSize(8 * MiB)

	if config.Policy.Classes.Sizes[0] != originalConfigFirstClass {
		t.Fatalf("mutating effective policy changed config policy: got %s, want %s",
			config.Policy.Classes.Sizes[0],
			originalConfigFirstClass,
		)
	}

	if second.Classes.Sizes[0] != originalSecondFirstClass {
		t.Fatalf("mutating first effective policy changed second effective policy: got %s, want %s",
			second.Classes.Sizes[0],
			originalSecondFirstClass,
		)
	}
}

// TestPoolPolicyValidationModeString verifies stable diagnostic labels for
// policy-validation modes.
func TestPoolPolicyValidationModeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode PoolPolicyValidationMode
		want string
	}{
		{
			name: "unset",
			mode: PoolPolicyValidationModeUnset,
			want: "unset",
		},
		{
			name: "enabled",
			mode: PoolPolicyValidationModeEnabled,
			want: "enabled",
		},
		{
			name: "disabled",
			mode: PoolPolicyValidationModeDisabled,
			want: "disabled",
		},
		{
			name: "unknown",
			mode: PoolPolicyValidationMode(255),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.mode.String(); got != tt.want {
				t.Fatalf("PoolPolicyValidationMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPoolCloseModeString verifies stable diagnostic labels for close modes.
func TestPoolCloseModeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode PoolCloseMode
		want string
	}{
		{
			name: "unset",
			mode: PoolCloseModeUnset,
			want: "unset",
		},
		{
			name: "clear retained",
			mode: PoolCloseModeClearRetained,
			want: "clear_retained",
		},
		{
			name: "keep retained",
			mode: PoolCloseModeKeepRetained,
			want: "keep_retained",
		},
		{
			name: "unknown",
			mode: PoolCloseMode(255),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.mode.String(); got != tt.want {
				t.Fatalf("PoolCloseMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestPoolClosedOperationModeString verifies stable diagnostic labels for
// post-close operation behavior.
func TestPoolClosedOperationModeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode PoolClosedOperationMode
		want string
	}{
		{
			name: "unset",
			mode: PoolClosedOperationModeUnset,
			want: "unset",
		},
		{
			name: "reject",
			mode: PoolClosedOperationModeReject,
			want: "reject",
		},
		{
			name: "drop returns",
			mode: PoolClosedOperationModeDropReturns,
			want: "drop_returns",
		},
		{
			name: "unknown",
			mode: PoolClosedOperationMode(255),
			want: "unknown",
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.mode.String(); got != tt.want {
				t.Fatalf("PoolClosedOperationMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// invalidPoolConfigPolicy returns a non-zero policy that fails validation.
//
// The policy is derived from DefaultPolicy so the failure is narrow and stable:
// soft retained bytes exceed hard retained bytes.
func invalidPoolConfigPolicy() Policy {
	policy := DefaultPolicy()
	policy.Retention.SoftRetainedBytes = policy.Retention.HardRetainedBytes + Byte

	return policy
}
