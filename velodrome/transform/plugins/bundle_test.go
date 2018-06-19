/*
Copyright 2017 The Kubernetes Authors.

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

package plugins

import (
	"strconv"
	"testing"
	"time"
)

func TestBundle(t *testing.T) {
	bundle := NewBundledStates("merged")
	if gotCount, gotSum := bundle.Total(time.Unix(0, 10)); gotCount != 0 || gotSum != 0 {
		t.Errorf("bundle.Total(time.Unix(0, 10)) = (%d, %d), want (0, 0)", gotCount, gotSum)
	}
	if got := bundle.Percentile(time.Unix(0, 10), 50); got != 0 {
		t.Errorf("bundle.Percentile(time.Unix(0, 10), 50) = %s, want 0", got)
	}

	// Enable fifty issues, each at 1 minute interval. State changes every time
	for i := 0; i < 50; i++ {
		if !bundle.ReceiveEvent(strconv.Itoa(i), "merged", "", time.Unix(60*int64(i), 0)) {
			t.Error("ReceiveEvent must have triggered a state change.")
		}
	}

	// we have 50 triggered state
	wantCount := 50
	// Total age at time 5000 is: (50*51)/2
	wantSum := int64(1275)
	if gotCount, gotSum := bundle.Total(time.Unix(50*60, 0)); gotCount != wantCount || gotSum != wantSum {
		t.Errorf("bundle.Total() = (%d, %d), want (%d, %d)", gotCount, gotSum, wantCount, wantSum)
	}
	// The issue in the middle has been opened for 25 minutes
	want := 25 * time.Minute
	if got := bundle.Percentile(time.Unix(50*60, 0), 50); got != want {
		t.Errorf("bundle.Percentile(time.Unix(50*60, 0), 50) = %s, want %s", got, want)
	}
}
