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

// Signed is the set of signed integer types supported by mathx numeric helpers.
//
// The tilde form accepts both predeclared integer types and project-defined
// numeric types with the same underlying representation. This is important for
// bufferpool internals because runtime values may later be represented by
// explicit domain types, for example:
//
//	type BufferCount int64
//	type ClassIndex int
//	type BudgetDelta int64
//
// Such types should remain usable with low-level math helpers without forcing
// callers to convert them back to raw built-in integers.
type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

// Unsigned is the set of unsigned integer types supported by mathx numeric
// helpers.
//
// Unsigned values are used for byte counts, capacities, retained memory limits,
// buffer counts, shard indexes, segment sizes, and other non-negative runtime
// quantities. uintptr is included because it is an unsigned integer type, but
// bufferpool code SHOULD use explicit integer-sized domain types for ordinary
// memory accounting instead of pointer-sized arithmetic unless pointer-sized
// behavior is required.
type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// Float is the set of floating-point types supported by mathx numeric helpers.
//
// Floating-point values are used by analytical control-plane calculations such
// as ratios, scores, EWMA-derived factors, retention efficiency, and pressure
// multipliers.
//
// Clamp does not validate NaN or infinity. Code that creates floating-point
// values MUST validate or normalize them at the source of the calculation, such
// as ratio, scoring, decay, or budget math. This keeps Clamp focused on one
// responsibility: bounding an already valid numeric value.
type Float interface {
	~float32 | ~float64
}

// Number is the numeric type set accepted by generic mathx helpers.
//
// Number intentionally excludes string even though strings are ordered in Go.
// This package is a numeric runtime utility layer, not a general ordering
// utility layer. Allowing string would make Clamp semantically too broad and
// would permit accidental use in code where only byte counts, capacities,
// counters, indexes, durations, ratios, or scores are valid.
//
// Number also excludes complex64 and complex128 because complex numbers are not
// ordered and therefore cannot be clamped with less-than / greater-than
// comparisons.
type Number interface {
	Signed | Unsigned | Float
}

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
