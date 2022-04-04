/*
Copyright 2022 The Kubernetes Authors.

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

package ghcache

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestCalculateRequestWaitDuration(t *testing.T) {
	maxDelayTime := time.Second * 10
	throttlingTime := time.Second
	throttlingTimeForGET := time.Millisecond * 100
	currentTime := time.Date(2022, time.January, 2, 0, 0, 0, 0, time.UTC)
	type args struct {
		t       tokenInfo
		toQueue time.Time
		getReq  bool
	}
	tests := []struct {
		name     string
		args     args
		toQueue  time.Time
		duration time.Duration
	}{
		{
			name: "No request for some time, no need to wait",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(-time.Minute),
				},
				toQueue: currentTime,
			},
			toQueue:  currentTime,
			duration: 0,
		},
		{
			name: "Non-GET request was made half second ago and incoming non-GET",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(-time.Millisecond * 500),
				},
				toQueue: currentTime,
			},
			toQueue:  currentTime.Add(time.Millisecond * 500),
			duration: time.Millisecond * 500,
		},
		{
			name: "GET request was made half second ago and incoming GET",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(-time.Millisecond * 500),
					getReq:    true,
				},
				toQueue: currentTime,
				getReq:  true,
			},
			toQueue:  currentTime,
			duration: 0,
		},
		{
			name: "Non-GET request needs to be scheduled, but there is a queue formed, adding on top",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(time.Second),
				},
				toQueue: currentTime,
			},
			toQueue:  currentTime.Add(2 * time.Second),
			duration: 2 * time.Second,
		},
		{
			name: "GET request needs to be scheduled, but there is a queue formed, adding on top",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(time.Second),
				},
				toQueue: currentTime,
				getReq:  true,
			},
			toQueue:  currentTime.Add(time.Second + throttlingTimeForGET),
			duration: time.Second + throttlingTimeForGET,
		},
		{
			name: "Non-GET request needs to be scheduled, but there is a large queue formed, adding with max schedule time",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(maxDelayTime - time.Millisecond),
				},
				toQueue: currentTime,
			},
			toQueue:  currentTime.Add(maxDelayTime),
			duration: maxDelayTime,
		},
		{
			name: "GET request needs to be scheduled, but there is a large queue formed, adding with max schedule time",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(maxDelayTime - time.Millisecond),
					getReq:    true,
				},
				toQueue: currentTime,
			},
			toQueue:  currentTime.Add(maxDelayTime),
			duration: maxDelayTime,
		},
		{
			name: "GET request needs to be scheduled, previous non-GET, duration shorter than throttlingTimeForGET",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(-throttlingTimeForGET / 2),
				},
				toQueue: currentTime,
				getReq:  true,
			},
			toQueue:  currentTime.Add(throttlingTimeForGET / 2),
			duration: throttlingTimeForGET / 2,
		},
		{
			name: "GET request was made and incoming request is also GET",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(-time.Millisecond * 50),
					getReq:    true,
				},
				toQueue: currentTime,
				getReq:  true,
			},
			toQueue:  currentTime.Add(time.Millisecond * 50),
			duration: time.Millisecond * 50,
		},
		{
			name: "GET request was made and incoming request is also GET, but time has passed",
			args: args{
				t: tokenInfo{
					timestamp: currentTime.Add(-throttlingTimeForGET * 2),
					getReq:    true,
				},
				toQueue: currentTime,
				getReq:  true,
			},
			toQueue:  currentTime,
			duration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &tokensRegistry{
				lock:                 sync.Mutex{},
				tokens:               map[string]tokenInfo{},
				throttlingTime:       throttlingTime,
				throttlingTimeForGET: throttlingTimeForGET,
				maxDelayTime:         maxDelayTime,
			}
			got, got1 := tr.calculateRequestWaitDuration(tt.args.t, tt.args.toQueue, tt.args.getReq)
			if !reflect.DeepEqual(got, tt.toQueue) {
				t.Errorf("calculateRequestWaitDuration() got = %v, want %v", got, tt.toQueue)
			}
			if got1 != tt.duration {
				t.Errorf("calculateRequestWaitDuration() got1 = %v, want %v", got1, tt.duration)
			}
		})
	}
}
