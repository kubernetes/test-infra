/*
Copyright 2016 The Kubernetes Authors.

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

package mungerutil

import (
	"sync"
	"time"
)

// FirstLabelTimeGetter represents anything that can get a label's first time.
// (interface is for testability / reduced import tree.)
type FirstLabelTimeGetter interface {
	FirstLabelTime(label string) *time.Time
	Number() int
}

// LabelTimeCache just caches the result of a time lookup, since the first time
// a label is applied never changes.
type LabelTimeCache struct {
	label string
	cache map[int]time.Time
	lock  sync.Mutex
}

// NewLabelTimeCache constructs a label time cache for the given label.
func NewLabelTimeCache(label string) *LabelTimeCache {
	return &LabelTimeCache{
		label: label,
		cache: map[int]time.Time{},
	}
}

// FirstLabelTime returns a time from the cache if possible. Otherwise, it
// will look up the time. If that doesn't work either, it will return
// (time.Time{}, false).
func (c *LabelTimeCache) FirstLabelTime(obj FirstLabelTimeGetter) (time.Time, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if t, ok := c.cache[obj.Number()]; ok {
		return t, true
	}
	t := obj.FirstLabelTime(c.label)
	if t == nil {
		return time.Time{}, false
	}
	c.cache[obj.Number()] = *t
	return *t, true
}
