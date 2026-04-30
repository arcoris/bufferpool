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
	"testing"
)

// TestPolicyProfilesValidateForManagedContext verifies that every public
// profile is usable by the managed PoolGroup/PoolPartition path.
func TestPolicyProfilesValidateForManagedContext(t *testing.T) {
	t.Parallel()

	for _, tt := range policyProfileFactories() {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if err := ValidatePolicyForContext(tt.policy(), PolicyValidationContextPoolGroup); err != nil {
				t.Fatalf("%s ValidatePolicyForContext(pool group) returned error: %v", tt.name, err)
			}
		})
	}
}

// TestPolicyProfilesValidateForStandaloneContext verifies that raw Pool mode
// accepts only profiles whose ownership posture can be enforced without
// LeaseRegistry.
func TestPolicyProfilesValidateForStandaloneContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		policy        func() Policy
		wantSupported bool
	}{
		{name: "DefaultPolicy", policy: DefaultPolicy, wantSupported: true},
		{name: "ThroughputPolicy", policy: ThroughputPolicy, wantSupported: true},
		{name: "MemoryConstrainedPolicy", policy: MemoryConstrainedPolicy, wantSupported: true},
		{name: "BurstyPolicy", policy: BurstyPolicy, wantSupported: true},
		{name: "StrictBoundedPolicy", policy: StrictBoundedPolicy, wantSupported: false},
		{name: "SecurePolicy", policy: SecurePolicy, wantSupported: false},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePolicyForContext(tt.policy(), PolicyValidationContextStandalonePool)
			if tt.wantSupported {
				if err != nil {
					t.Fatalf("%s standalone context returned error: %v", tt.name, err)
				}
				return
			}

			if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("%s standalone context error = %v, want ErrInvalidPolicy", tt.name, err)
			}
		})
	}
}
