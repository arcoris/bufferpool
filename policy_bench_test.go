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

import "testing"

// BenchmarkPolicyValidateDefault measures validation of the canonical default
// policy, including section-local and cross-section checks.
func BenchmarkPolicyValidateDefault(b *testing.B) {
	policy := DefaultPolicy()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := policy.Validate(); err != nil {
			b.Fatalf("Policy.Validate() returned error: %v", err)
		}
	}
}

// BenchmarkPolicyValidateProfiles measures validation across all named
// profiles as a compact benchmark matrix for profile defaults.
func BenchmarkPolicyValidateProfiles(b *testing.B) {
	profiles := policyProfileFactories()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, profile := range profiles {
			if err := profile.policy().Validate(); err != nil {
				b.Fatalf("%s Validate() returned error: %v", profile.name, err)
			}
		}
	}
}

// BenchmarkPolicyValidateManagedContext measures policy validation with managed
// PoolGroup/PoolPartition support checks.
func BenchmarkPolicyValidateManagedContext(b *testing.B) {
	policy := SecurePolicy()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := ValidatePolicyForContext(policy, PolicyValidationContextPoolGroup); err != nil {
			b.Fatalf("ValidatePolicyForContext() returned error: %v", err)
		}
	}
}

// BenchmarkPolicySectionProjection measures the cost of splitting a Policy into
// construction-shape and runtime-retention sections.
func BenchmarkPolicySectionProjection(b *testing.B) {
	policy := DefaultPolicy()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		shape := policy.ShapePolicy()
		retention := policy.RetentionPolicy()
		poolBenchmarkIntSink = len(shape.Classes.Sizes) + retention.Trim.MaxPoolsPerCycle
	}
}
