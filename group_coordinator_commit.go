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

// commitGroupCoordinatorCycle saves coordinator observation state.
//
// Group coordinator commits are intentionally broader than partition commits:
// a no-target Skipped cycle and a non-error Unpublished cycle still record the
// sample/window baseline because they completed group observation. Only returned
// publication errors skip this commit path.
func (g *PoolGroup) commitGroupCoordinatorCycle(cycle *groupCoordinatorCycleEvaluation) Generation {
	g.coordinator.previousSample = copyPoolGroupSampleInto(g.coordinator.previousSample, cycle.sample)
	g.coordinator.previousSampleTime = cycle.now
	g.coordinator.hasPreviousSample = true

	return g.coordinator.generation.Advance()
}
