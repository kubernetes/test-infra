/*
Copyright 2021 The Kubernetes Authors.

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

// Package secretutil implements utilities to operate on secret data.
package secretutil

import (
	"encoding/base64"
	"strings"
	"sync"

	"go4.org/bytereplacer"
)

// Censorer knows how to replace sensitive data from input.
type Censorer interface {
	// Censor will remove sensitive data previously registered with the Censorer
	// from the input. This is thread-safe, will mutate the input and will never
	// change the overall size of the input.
	Censor(input *[]byte)
}

func NewCensorer() *ReloadingCensorer {
	return &ReloadingCensorer{
		RWMutex:  &sync.RWMutex{},
		Replacer: bytereplacer.New(),
	}
}

type ReloadingCensorer struct {
	*sync.RWMutex
	*bytereplacer.Replacer
	largestSecret int
}

var _ Censorer = &ReloadingCensorer{}

// Censor will remove sensitive data previously registered with the Censorer
// from the input. This is thread-safe, will mutate the input and will never
// change the overall size of the input.
// Censoring will attempt to be intelligent about how content is removed from
// the input - when the ReloadingCensorer is given secrets to censor, we:
//  - handle the case where whitespace is needed to be trimmed
//  - censor not only the plaintext representation of the secret but also
//    the base64-encoded representation of it, as it's common for k8s
//    Secrets to contain information in this way
func (c *ReloadingCensorer) Censor(input *[]byte) {
	c.RLock()
	// we know our replacer will never have to allocate, as our replacements
	// are the same size as what they're replacing, so we can throw away
	// the return value from Replace()
	c.Replacer.Replace(*input)
	c.RUnlock()
}

// LargestSecret returns the size of the largest secret we will censor.
func (c *ReloadingCensorer) LargestSecret() int {
	c.RLock()
	defer c.RUnlock()
	return c.largestSecret
}

// RefreshBytes refreshes the set of secrets that we censor.
func (c *ReloadingCensorer) RefreshBytes(secrets ...[]byte) {
	var asStrings []string
	for _, secret := range secrets {
		asStrings = append(asStrings, string(secret))
	}
	c.Refresh(asStrings...)
}

// Refresh refreshes the set of secrets that we censor.
func (c *ReloadingCensorer) Refresh(secrets ...string) {
	var largestSecret int
	var replacements []string
	addReplacement := func(s string) {
		replacements = append(replacements, s, strings.Repeat(`*`, len(s)))
		if len(s) > largestSecret {
			largestSecret = len(s)
		}
	}
	for _, secret := range secrets {
		if trimmed := strings.TrimSpace(secret); trimmed != secret {
			secret = trimmed
		}
		if secret == "" {
			continue
		}
		addReplacement(secret)
		encoded := base64.StdEncoding.EncodeToString([]byte(secret))
		addReplacement(encoded)
	}
	c.Lock()
	c.Replacer = bytereplacer.New(replacements...)
	c.largestSecret = largestSecret
	c.Unlock()
}

// AdaptCensorer returns a func that censors without touching the input, to
// be used in places where the previous behavior is required while migrations
// occur.
func AdaptCensorer(censorer Censorer) func(input []byte) []byte {
	return func(input []byte) []byte {
		output := make([]byte, len(input))
		copy(output, input)
		censorer.Censor(&output)
		return output
	}
}
