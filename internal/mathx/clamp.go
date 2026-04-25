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

package mathx

const (
	// errClampInvalidRange is used when the caller provides an invalid inclusive
	// range where minValue is greater than maxValue.
	//
	// In bufferpool internals, invalid clamp bounds indicate a programming error:
	// configuration normalization, policy validation, budget calculation, or
	// runtime state preparation produced an impossible range. Such bugs MUST be
	// detected immediately instead of being silently corrected.
	errClampInvalidRange = "mathx.Clamp: minValue must be less than or equal to maxValue"
)

// Clamp returns value constrained to the inclusive numeric range
// [minValue, maxValue].
//
// Clamp is a low-level, allocation-free arithmetic primitive. It is intended for
// internal bufferpool runtime calculations where a computed numeric value must
// be bounded before it is applied to state.
//
// Typical bufferpool use cases include:
//
//   - bounding shard counts derived from runtime or configuration values;
//   - bounding partition, pool, class, and shard retained-memory targets;
//   - bounding segment sizes and buffer-count limits;
//   - bounding trim byte limits and trim buffer limits;
//   - bounding controller work limits;
//   - bounding already validated ratios, scores, and pressure multipliers.
//
// The range is inclusive:
//
//   - if value < minValue, Clamp returns minValue;
//   - if value > maxValue, Clamp returns maxValue;
//   - otherwise Clamp returns value unchanged.
//
// The caller MUST pass a valid range where minValue <= maxValue. An invalid
// range is treated as an internal programming error and causes a panic.
//
// Clamp deliberately accepts only Number, not cmp.Ordered. cmp.Ordered also
// permits strings, but strings are not valid inputs for bufferpool runtime math.
// Keeping the constraint numeric prevents accidental non-numeric use at compile
// time.
//
// For floating-point values, Clamp follows normal Go comparison semantics. NaN
// and infinity validation is intentionally outside this function. The producer
// of floating-point values, such as ratio, EWMA, score, or budget code, MUST
// ensure those values are valid before they reach Clamp.
func Clamp[T Number](value, minValue, maxValue T) T {
	if minValue > maxValue {
		panic(errClampInvalidRange)
	}

	if value < minValue {
		return minValue
	}

	if value > maxValue {
		return maxValue
	}

	return value
}
