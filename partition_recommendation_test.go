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

// TestPoolPartitionRecommendation verifies score-to-recommendation projection.
func TestPoolPartitionRecommendation(t *testing.T) {
	tests := []struct {
		name   string
		scores PoolPartitionScores
		want   PoolPartitionRecommendationKind
	}{
		{name: "low signal", want: PoolPartitionRecommendationObserve},
		{
			name: "high risk",
			scores: PoolPartitionScores{
				Risk: PoolPartitionRiskScore{Value: 0.9},
			},
			want: PoolPartitionRecommendationInvestigateOwnership,
		},
		{
			name: "pressure waste",
			scores: PoolPartitionScores{
				Pressure: PoolPartitionPressureScore{Value: 0.66},
				Waste:    PoolPartitionWasteScore{Value: 0.7},
			},
			want: PoolPartitionRecommendationTrim,
		},
		{
			name: "waste",
			scores: PoolPartitionScores{
				Waste: PoolPartitionWasteScore{Value: 0.8},
			},
			want: PoolPartitionRecommendationShrinkRetention,
		},
		{
			name: "usefulness",
			scores: PoolPartitionScores{
				Usefulness: PoolPartitionUsefulnessScore{Value: 0.9},
				Pressure:   PoolPartitionPressureScore{Value: 0.1},
			},
			want: PoolPartitionRecommendationGrowRetention,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPoolPartitionRecommendation(tt.scores)
			if got.Kind != tt.want {
				t.Fatalf("NewPoolPartitionRecommendation() = %+v, want kind %d", got, tt.want)
			}
			if got.Confidence < 0 || got.Confidence > 1 {
				t.Fatalf("confidence out of range: %+v", got)
			}
		})
	}
}
