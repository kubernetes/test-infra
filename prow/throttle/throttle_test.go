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
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestThrottle(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	t.Parallel()
	testCases := []struct {
		name string

		setup []struct {
			hourly int
			burst  int
			org    string
		}
		expectThrottling bool
	}{
		{
			name: "global throttler",
			setup: []struct {
				hourly int
				burst  int
				org    string
			}{
				{1, 2, ""},
			},
			expectThrottling: true,
		},
		{
			name: "our org is throttled",

			setup: []struct {
				hourly int
				burst  int
				org    string
			}{
				{1, 2, "org"},
			},
			expectThrottling: true,
		},
		{
			name: "different org is throttled, ours is not",

			setup: []struct {
				hourly int
				burst  int
				org    string
			}{
				{1, 2, "something-else"},
			},
		},
		{
			name: "global throttler and throttler for our org",

			setup: []struct {
				hourly int
				burst  int
				org    string
			}{
				{100, 100, ""},
				{1, 2, "org"},
			},
			expectThrottling: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			throttler := &Throttler{}
			throttlerKey := throttlerGlobalKey
			for _, setup := range tc.setup {
				if setup.org != "" {
					throttler.Throttle(setup.hourly, setup.burst, setup.org)
					if setup.org == "org" {
						throttlerKey = "org"
					}
				} else {
					throttler.Throttle(setup.hourly, setup.burst)
				}
			}

			var expectItems int
			if tc.expectThrottling {
				expectItems = 2
			}
			if n := len(throttler.throttle[throttlerKey]); n != expectItems {
				t.Fatalf("Expected %d items in throttle channel, found %d", expectItems, n)
			}
			if n := cap(throttler.throttle[throttlerKey]); n != expectItems {
				t.Fatalf("Expected throttle channel capacity of %d, found %d", expectItems, n)
			}
			check := func(err error) {
				t.Helper()
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if tc.expectThrottling {
					if len(throttler.throttle[throttlerKey]) != 1 {
						t.Errorf("Expected one item in throttle channel, found %d", len(throttler.throttle[throttlerKey]))
					}
				} else if _, throttleChannelExists := throttler.throttle[throttlerKey]; throttleChannelExists {
					t.Error("didn't expect throttling, but throttler existed")
				}
			}
			err := throttler.Wait(context.Background(), "org")
			check(err)
			// The following two waits should be properly refunded.
			err = throttler.Wait(context.Background(), "org")
			throttler.Refund("org")
			check(err)
			err = throttler.Wait(context.Background(), "org")
			throttler.Refund("org")
			check(err)

			// Check that calls are delayed while throttled.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			go func() {
				if err := throttler.Wait(context.Background(), "org"); err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if err := throttler.Wait(context.Background(), "org"); err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				cancel()
			}()
			slowed := false
			for ctx.Err() == nil {
				// Wait for the client to get throttled
				val := throttler.slow[throttlerKey]
				if val == nil || atomic.LoadInt32(val) == 0 {
					continue
				}
				// Throttled, now add to the channel
				slowed = true
				select {
				case throttler.throttle[throttlerKey] <- time.Now(): // Add items to the channel
				case <-ctx.Done():
				}
			}
			if slowed != tc.expectThrottling {
				t.Errorf("expected throttling: %t, got throttled: %t", tc.expectThrottling, slowed)
			}
			if err := ctx.Err(); err != context.Canceled {
				t.Errorf("Expected context cancellation did not happen: %v", err)
			}
		})
	}
}
