/*
Copyright 2020 The Kubernetes Authors.

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
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

type roundTripperCreator func(partitionKey string) http.RoundTripper

// partitioningRoundTripper is a http.RoundTripper
var _ http.RoundTripper = &partitioningRoundTripper{}

func newPartitioningRoundTripper(rtc roundTripperCreator) *partitioningRoundTripper {
	return &partitioningRoundTripper{
		roundTripperCreator: rtc,
		lock:                &sync.Mutex{},
		roundTrippers:       map[string]http.RoundTripper{},
	}
}

type partitioningRoundTripper struct {
	roundTripperCreator roundTripperCreator
	lock                *sync.Mutex
	roundTrippers       map[string]http.RoundTripper
}

func getCachePartition(r *http.Request) string {
	// Hash the key to make sure we dont leak it into the directory layout
	return fmt.Sprintf("%x", sha256.Sum256([]byte(r.Header.Get("Authorization"))))
}

func (prt *partitioningRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	cachePartition := getCachePartition(r)

	prt.lock.Lock()
	roundTripper, found := prt.roundTrippers[cachePartition]
	if !found {
		logrus.WithField("cache-parition-key", cachePartition).Info("Creating a new cache for partition")
		cachePartitionsCounter.WithLabelValues(cachePartition).Add(1)
		prt.roundTrippers[cachePartition] = prt.roundTripperCreator(cachePartition)
		roundTripper = prt.roundTrippers[cachePartition]
	}
	prt.lock.Unlock()

	return roundTripper.RoundTrip(r)
}
