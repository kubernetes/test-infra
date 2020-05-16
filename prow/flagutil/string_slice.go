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

package flagutil

import (
	"strings"
)

// StringSlice is a generic command-line flag for a []string.
// The flag is specified using the format: value1,value2,value3
type StringSlice struct {
	value []string
}

// NewStringSlice creates a new instance of StringSlice.
func NewStringSlice(value []string) *StringSlice {
	return &StringSlice{
		value: value,
	}
}

// Set returns the string representation of the StringSlice.
func (s *StringSlice) String() string {
	return strings.Join(s.value, ",")
}

// Strings gets the value of the StringSlice.
func (s *StringSlice) Get() []string {
	return s.value
}

// Set sets the value of the StringSlice.
func (s *StringSlice) Set(value string) error {
	s.value = strings.Split(value, ",")
	return nil
}
