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

import (
	"time"

	"arcoris.dev/bufferpool/internal/multierr"
)

const (
	// defaultPartitionDrainPollInterval bounds graceful-close polling cadence.
	defaultPartitionDrainPollInterval = time.Millisecond

	// errPartitionDrainNegativeTimeout reports an invalid graceful-close timeout.
	errPartitionDrainNegativeTimeout = "bufferpool.PoolPartitionDrainPolicy: timeout must not be negative"

	// errPartitionDrainNegativePollInterval reports an invalid poll interval.
	errPartitionDrainNegativePollInterval = "bufferpool.PoolPartitionDrainPolicy: poll interval must not be negative"
)

// PoolPartitionDrainPolicy configures bounded graceful partition close.
//
// Graceful close is foreground polling, not a background drain controller. It
// closes the partition LeaseRegistry to reject new acquisitions, waits for
// active leases to release naturally, and closes Pools only after active lease
// ownership reaches zero. A zero Timeout performs one non-blocking observation.
type PoolPartitionDrainPolicy struct {
	// Timeout is the maximum time to wait for active leases to drain.
	Timeout time.Duration

	// PollInterval controls how often active lease counters are sampled.
	PollInterval time.Duration
}

// Normalize returns p with graceful-drain defaults completed.
func (p PoolPartitionDrainPolicy) Normalize() PoolPartitionDrainPolicy {
	if p.PollInterval == 0 {
		p.PollInterval = defaultPartitionDrainPollInterval
	}
	return p
}

// Validate validates graceful-drain timing settings.
func (p PoolPartitionDrainPolicy) Validate() error {
	if p.Timeout < 0 {
		return newError(ErrInvalidOptions, errPartitionDrainNegativeTimeout)
	}
	if p.PollInterval < 0 {
		return newError(ErrInvalidOptions, errPartitionDrainNegativePollInterval)
	}
	return nil
}

// PoolPartitionDrainResult describes one graceful close attempt.
type PoolPartitionDrainResult struct {
	// Attempted reports whether this call started or joined graceful shutdown.
	Attempted bool

	// Completed reports whether active leases reached zero and Pools were closed.
	Completed bool

	// TimedOut reports whether Timeout elapsed while leases were still active.
	TimedOut bool

	// ActiveLeasesBefore is the active lease count after new acquisitions were
	// rejected.
	ActiveLeasesBefore int

	// ActiveLeasesAfter is the active lease count at completion or timeout.
	ActiveLeasesAfter int
}

// CloseGracefully performs a bounded graceful partition close.
//
// CloseGracefully preserves the hard Close contract by adding a separate API
// instead of changing Close. It never force-releases active buffers and never
// closes Pools before the partition-owned LeaseRegistry reports zero active
// leases. On timeout, the partition remains closing with the LeaseRegistry
// closed to new acquisitions; callers may retry CloseGracefully or call Close
// for hard shutdown. Already-closed partitions return Attempted=false because no
// new drain attempt starts. Final Pool cleanup is serialized with hard Close so
// concurrent graceful and hard shutdown paths cannot close child Pools twice.
func (p *PoolPartition) CloseGracefully(policy PoolPartitionDrainPolicy) (PoolPartitionDrainResult, error) {
	p.mustBeInitialized()
	policy = policy.Normalize()
	if err := policy.Validate(); err != nil {
		return PoolPartitionDrainResult{}, err
	}

	if p.lifecycle.IsClosed() {
		active := p.activeLeaseCount()
		return PoolPartitionDrainResult{Completed: active == 0, ActiveLeasesBefore: active, ActiveLeasesAfter: active}, nil
	}

	if !p.lifecycle.IsClosing() {
		p.lifecycle.BeginClose()
	}

	var err error
	if closeErr := p.leases.Close(); closeErr != nil {
		multierr.AppendInto(&err, closeErr)
	}

	result := PoolPartitionDrainResult{Attempted: true}
	result.ActiveLeasesBefore = p.activeLeaseCount()

	deadline := time.Now().Add(policy.Timeout)
	for {
		active := p.activeLeaseCount()
		result.ActiveLeasesAfter = active
		if active == 0 {
			p.closeMu.Lock()
			defer p.closeMu.Unlock()

			if p.lifecycle.IsClosed() {
				result.Completed = true
				return result, err
			}
			if closeErr := p.registry.closeAll(); closeErr != nil {
				multierr.AppendInto(&err, closeErr)
			}
			p.lifecycle.MarkClosed()
			p.generation.Advance()
			result.Completed = true
			return result, err
		}

		if policy.Timeout == 0 || !time.Now().Before(deadline) {
			result.TimedOut = true
			return result, err
		}

		sleep := policy.PollInterval
		if remaining := time.Until(deadline); remaining < sleep {
			sleep = remaining
		}
		if sleep <= 0 {
			result.TimedOut = true
			return result, err
		}
		time.Sleep(sleep)
	}
}

// activeLeaseCount samples the partition-owned LeaseRegistry active count.
func (p *PoolPartition) activeLeaseCount() int {
	var sample leaseCounterSample
	p.leases.sampleCounters(&sample)
	return sample.ActiveCount
}
