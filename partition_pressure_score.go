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

import controlpressure "arcoris.dev/bufferpool/internal/control/pressure"

// PoolPartitionPressureScore maps partition pressure to normalized severity.
type PoolPartitionPressureScore struct {
	// Value is the normalized pressure severity.
	Value float64

	// Level is the observed domain pressure level.
	Level PressureLevel

	// Enabled reports whether pressure interpretation was enabled.
	Enabled bool
}

// newPoolPartitionPressureScore maps root pressure levels to shared severity.
//
// The mapping is intentionally explicit instead of relying on numeric enum
// equality, which keeps the internal pressure package independent from root
// PressureLevel values. Disabled pressure contributes zero severity even if the
// snapshot carries a non-normal level; domain validation should normally keep
// disabled pressure normal, but projection remains defensive.
func newPoolPartitionPressureScore(snapshot PartitionPressureSnapshot) PoolPartitionPressureScore {
	if !snapshot.Enabled {
		return PoolPartitionPressureScore{
			Value:   0,
			Level:   snapshot.Level,
			Enabled: false,
		}
	}

	level := controlpressure.LevelNormal
	switch snapshot.Level {
	case PressureLevelMedium:
		level = controlpressure.LevelMedium
	case PressureLevelHigh:
		level = controlpressure.LevelHigh
	case PressureLevelCritical:
		level = controlpressure.LevelCritical
	}
	return PoolPartitionPressureScore{
		Value:   controlpressure.Severity(level),
		Level:   snapshot.Level,
		Enabled: snapshot.Enabled,
	}
}
