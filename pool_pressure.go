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
	errPoolPressureInvalid = "bufferpool.Pool: invalid pressure signal"
)

// applyPressure publishes pressure into Pool's immutable runtime snapshot.
//
// Pool does not compute pressure and does not call controllers from Get or Put.
// A higher owner publishes the signal, and the return path reads that immutable
// snapshot before shard credit or bucket storage is touched.
func (p *Pool) applyPressure(signal PressureSignal) error {
	p.mustBeInitialized()
	if err := signal.validate(); err != nil {
		return wrapError(ErrInvalidOptions, err, errPoolPressureInvalid)
	}

	runtime := p.currentRuntimeSnapshot()
	generation := budgetPublicationGeneration(runtime.Generation, signal.Generation)
	p.publishRuntimeSnapshot(newPoolRuntimeSnapshotWithPressure(generation, runtime.Policy, signal))
	return nil
}

// poolPressureReturnOutcome applies runtime pressure admission before a return
// reaches class routing, shard credit, or bucket storage.
func poolPressureReturnOutcome(input poolReturnInput, policy Policy, signal PressureSignal) (poolReturnOutcome, bool) {
	if !policy.Pressure.Enabled || signal.Level == PressureLevelNormal {
		return poolReturnOutcome{}, false
	}

	levelPolicy := pressureLevelPolicy(policy.Pressure, signal.Level)
	if levelPolicy.DisableRetention {
		return poolReturnOutcomeDrop(
			AdmissionActionDrop,
			nil,
			"",
			PoolDropReasonPressureRetentionDisabled,
			input.Capacity,
		), true
	}

	if !levelPolicy.DropReturnedCapacityAbove.IsZero() && input.Capacity > levelPolicy.DropReturnedCapacityAbove.Bytes() {
		return poolReturnOutcomeDrop(
			policy.Admission.OversizedReturn,
			ErrBufferTooLarge,
			errPoolPutBufferTooLarge,
			PoolDropReasonPressureCapacityThreshold,
			input.Capacity,
		), true
	}

	if !levelPolicy.MaxRetainedBufferCapacity.IsZero() && input.Capacity > levelPolicy.MaxRetainedBufferCapacity.Bytes() {
		return poolReturnOutcomeDrop(
			policy.Admission.OversizedReturn,
			ErrBufferTooLarge,
			errPoolPutBufferTooLarge,
			PoolDropReasonPressureCapacityThreshold,
			input.Capacity,
		), true
	}

	return poolReturnOutcome{}, false
}
