/*
Copyright 2019 The Kubernetes Authors.

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

package ghmetrics

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

// Hasher knows how to hash an authorization header from a request
type Hasher interface {
	Hash(req *http.Request) string
}

func NewCachingHasher() Hasher {
	return &cachingHasher{
		lock:   sync.RWMutex{},
		hashes: map[string]string{},
	}
}

type cachingHasher struct {
	lock   sync.RWMutex
	hashes map[string]string
}

func (h *cachingHasher) Hash(req *http.Request) string {
	// get authorization header to convert to sha256
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		logrus.Warn("Couldn't retrieve 'Authorization' header, adding to unknown bucket")
		authHeader = "unknown"
	}
	h.lock.RLock()
	hash, cached := h.hashes[authHeader]
	h.lock.RUnlock()
	if cached {
		return hash
	}

	h.lock.Lock()
	hash = fmt.Sprintf("%x", sha256.Sum256([]byte(authHeader))) // use %x to make this a utf-8 string for use as a label
	h.hashes[authHeader] = hash
	h.lock.Unlock()
	return hash
}
