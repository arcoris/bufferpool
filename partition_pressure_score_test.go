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

func TestPoolPartitionPressureScoreDisabledIsZero(t *testing.T) {
	score := newPoolPartitionPressureScore(PartitionPressureSnapshot{
		Enabled: false,
		Level:   PressureLevelCritical,
	})

	if score.Value != 0 || score.Enabled {
		t.Fatalf("disabled pressure score = %+v, want zero disabled score", score)
	}
	if score.Level != PressureLevelCritical {
		t.Fatalf("disabled pressure level = %v, want original snapshot level", score.Level)
	}
}

func TestPoolPartitionPressureScoreEnabledSeverity(t *testing.T) {
	score := newPoolPartitionPressureScore(PartitionPressureSnapshot{
		Enabled: true,
		Level:   PressureLevelCritical,
	})

	if score.Value != 1 || !score.Enabled || score.Level != PressureLevelCritical {
		t.Fatalf("enabled pressure score = %+v, want critical severity", score)
	}
}
