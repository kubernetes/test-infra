/*
Copyright 2023 The Kubernetes Authors.

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

package throttle

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

const throttlerGlobalKey = "*"

type Throttler struct {
	ticker   map[string]*time.Ticker
	throttle map[string]chan time.Time
	slow     map[string]*int32 // Helps log once when requests start/stop being throttled
	lock     sync.RWMutex
}

func (t *Throttler) Wait(ctx context.Context, org string) error {
	start := time.Now()
	log := logrus.WithFields(logrus.Fields{"throttled": true})
	defer func() {
		waitTime := time.Since(start)
		switch {
		case waitTime > 15*time.Minute:
			log.WithField("throttle-duration", waitTime.String()).Warn("Throttled clientside for more than 15 minutes")
		case waitTime > time.Minute:
			log.WithField("throttle-duration", waitTime.String()).Debug("Throttled clientside for more than a minute")
		}
	}()
	t.lock.RLock()
	defer t.lock.RUnlock()
	if _, found := t.ticker[org]; !found {
		org = throttlerGlobalKey
	}
	if _, hasThrottler := t.ticker[org]; !hasThrottler {
		return nil
	}

	var more bool
	select {
	case _, more = <-t.throttle[org]:
		// If we were throttled and the channel is now somewhat (25%+) full, note this
		if len(t.throttle[org]) > cap(t.throttle[org])/4 && atomic.CompareAndSwapInt32(t.slow[org], 1, 0) {
			log.Debug("Unthrottled")
		}
		if !more {
			log.Debug("Throttle channel closed")
		}
		return nil
	default: // Do not wait if nothing is available right now
	}
	// If this is the first time we are waiting, note this
	if slow := atomic.SwapInt32(t.slow[org], 1); slow == 0 {
		log.Debug("Throttled")
	}

	select {
	case _, more = <-t.throttle[org]:
		if !more {
			log.Debug("Throttle channel closed")
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (t *Throttler) Refund(org string) {
	t.lock.RLock()
	defer t.lock.RUnlock()
	if _, found := t.ticker[org]; !found {
		org = throttlerGlobalKey
	}
	if _, hasThrottler := t.ticker[org]; !hasThrottler {
		return
	}
	select {
	case t.throttle[org] <- time.Now():
	default:
	}
}

// Throttle client to a rate of at most hourlyTokens requests per hour,
// allowing burst tokens.
func (t *Throttler) Throttle(hourlyTokens, burst int, orgs ...string) error {
	org := "*"
	if len(orgs) > 0 {
		if len(orgs) > 1 {
			return fmt.Errorf("may only pass one org for throttling, got %d", len(orgs))
		}
		org = orgs[0]
	}
	t.lock.Lock()
	defer t.lock.Unlock()
	if hourlyTokens <= 0 || burst <= 0 { // Disable throttle
		if t.throttle[org] != nil {
			delete(t.throttle, org)
			delete(t.slow, org)
			t.ticker[org].Stop()
			delete(t.ticker, org)
		}
		return nil
	}
	period := time.Hour / time.Duration(hourlyTokens) // Duration between token refills
	ticker := time.NewTicker(period)
	throttle := make(chan time.Time, burst)
	for i := 0; i < burst; i++ { // Fill up the channel
		throttle <- time.Now()
	}
	go func() {
		// Before refilling, wait the amount of time it would have taken to refill the burst channel.
		// This prevents granting too many tokens in the first hour due to the initial burst.
		for i := 0; i < burst; i++ {
			<-ticker.C
		}
		// Refill the channel
		for t := range ticker.C {
			select {
			case throttle <- t:
			default:
			}
		}
	}()

	if t.ticker == nil {
		t.ticker = map[string]*time.Ticker{}
	}
	t.ticker[org] = ticker

	if t.throttle == nil {
		t.throttle = map[string]chan time.Time{}
	}
	t.throttle[org] = throttle

	if t.slow == nil {
		t.slow = map[string]*int32{}
	}
	var i int32
	t.slow[org] = &i

	return nil
}
