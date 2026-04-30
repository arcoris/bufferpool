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

const (
	errPressureInvalidLevel = "bufferpool.Pressure: unknown pressure level"
)

// PressureSource identifies the owner that published a pressure signal.
type PressureSource uint8

const (
	// PressureSourceUnset means no publisher was recorded.
	PressureSourceUnset PressureSource = iota

	// PressureSourceManual means a foreground API call published the signal.
	PressureSourceManual

	// PressureSourceGroup means PoolGroup propagated the signal to partitions.
	PressureSourceGroup

	// PressureSourcePartition means PoolPartition propagated the signal to Pools.
	PressureSourcePartition
)

// PressureSignal is an immutable runtime pressure publication.
type PressureSignal struct {
	// Level is the current pressure level.
	Level PressureLevel

	// Source identifies the publisher.
	Source PressureSource

	// Generation identifies the publication event.
	Generation Generation
}

// IsZero reports whether s contains no meaningful pressure publication.
func (s PressureSignal) IsZero() bool {
	return s.Level == PressureLevelNormal && s.Source == PressureSourceUnset && s.Generation.IsZero()
}

// validate validates a pressure signal before publication.
func (s PressureSignal) validate() error {
	if !isKnownPressureLevel(s.Level) {
		return newError(ErrInvalidOptions, errPressureInvalidLevel)
	}
	return nil
}

// normalPressureSignal returns the default non-contracting pressure signal.
func normalPressureSignal(generation Generation) PressureSignal {
	return PressureSignal{Level: PressureLevelNormal, Generation: generation}
}

// isKnownPressureLevel reports whether level is one of the supported runtime
// pressure levels.
func isKnownPressureLevel(level PressureLevel) bool {
	switch level {
	case PressureLevelNormal, PressureLevelMedium, PressureLevelHigh, PressureLevelCritical:
		return true
	default:
		return false
	}
}

// pressureLevelPolicy returns the behavior configured for one pressure level.
func pressureLevelPolicy(policy PressurePolicy, level PressureLevel) PressureLevelPolicy {
	switch level {
	case PressureLevelMedium:
		return policy.Medium
	case PressureLevelHigh:
		return policy.High
	case PressureLevelCritical:
		return policy.Critical
	default:
		return PressureLevelPolicy{}
	}
}
