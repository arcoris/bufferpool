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

// PoolGroupCounterDelta is the aggregate counter movement between two group samples.
//
// It intentionally reuses the partition counter-delta shape because a group
// aggregate sample is partition-shaped. This keeps group rate projection aligned
// with the already hardened partition rate semantics.
type PoolGroupCounterDelta struct {
	// Aggregate is the raw counter movement across the aggregate group sample.
	Aggregate PoolPartitionCounterDelta
}

// PoolGroupWindow captures two group samples and their aggregate counter delta.
//
// Generation and PolicyGeneration come from Current. Window construction does
// not mutate PoolGroup, PoolPartition, Pool, or LeaseRegistry state.
type PoolGroupWindow struct {
	// Generation is the current sample group generation.
	Generation Generation

	// PolicyGeneration is the current sample runtime-policy generation.
	PolicyGeneration Generation

	// Previous is the older sample used for delta computation.
	Previous PoolGroupSample

	// Current is the newer sample used for delta computation.
	Current PoolGroupSample

	// Delta is the raw aggregate counter movement from Previous to Current.
	Delta PoolGroupCounterDelta
}

// NewPoolGroupWindow returns a window from previous to current.
func NewPoolGroupWindow(previous, current PoolGroupSample) PoolGroupWindow {
	var window PoolGroupWindow
	window.Reset(previous, current)
	return window
}

// Reset rebuilds w from previous to current while reusing stored sample slices.
//
// Reset owns copies of Previous and Current so caller mutations after Reset do
// not affect the window. It allocates only when the existing stored partition or
// nested Pool sample capacity is too small for the input samples.
func (w *PoolGroupWindow) Reset(previous, current PoolGroupSample) {
	if w == nil {
		return
	}
	w.Generation = current.Generation
	w.PolicyGeneration = current.PolicyGeneration
	w.Previous = copyPoolGroupSampleInto(w.Previous, previous)
	w.Current = copyPoolGroupSampleInto(w.Current, current)
	w.Delta = newPoolGroupCounterDelta(previous, current)
}

// newPoolGroupCounterDelta computes aggregate counter movement between samples.
func newPoolGroupCounterDelta(previous, current PoolGroupSample) PoolGroupCounterDelta {
	partitionWindow := NewPoolPartitionWindow(previous.Aggregate, current.Aggregate)
	return PoolGroupCounterDelta{Aggregate: partitionWindow.Delta}
}

// clonePoolGroupSample returns a group sample copy with owned slice storage.
func clonePoolGroupSample(sample PoolGroupSample) PoolGroupSample {
	return copyPoolGroupSampleInto(PoolGroupSample{}, sample)
}

// copyPoolGroupSampleInto copies src into dst while reusing destination capacity.
//
// Reuse includes the nested PoolPartitionSample.Pools slices inside each
// partition sample and Aggregate. That keeps Reset allocation-conscious while
// still owning all copied sample storage.
func copyPoolGroupSampleInto(dst, src PoolGroupSample) PoolGroupSample {
	partitions := dst.Partitions[:0]
	if src.Partitions != nil {
		if cap(partitions) < len(src.Partitions) {
			partitions = make([]PoolGroupPartitionSample, len(src.Partitions))
		} else {
			partitions = partitions[:len(src.Partitions)]
		}
		for index, partition := range src.Partitions {
			previousSample := partitions[index].Sample
			partitions[index] = partition
			partitions[index].Sample = copyPoolPartitionSampleInto(previousSample, partition.Sample)
		}
	} else {
		partitions = nil
	}
	aggregate := copyPoolPartitionSampleInto(dst.Aggregate, src.Aggregate)
	dst = src
	dst.Partitions = partitions
	dst.Aggregate = aggregate
	return dst
}
