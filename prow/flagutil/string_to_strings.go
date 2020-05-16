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
	"fmt"
	"strings"
)

// StringToStringSlice is a generic command-line flag for a map[string][]string.
// The flag is specified using the format: key1=value1,value2;key2=value1,value2
type StringToStringSlice struct {
	value map[string][]string
}

// NewStringToStringSlice creates a new instance of StringToStringSlice.
func NewStringToStringSlice(value map[string][]string) *StringToStringSlice {
	return &StringToStringSlice{
		value: value,
	}
}

// Set returns the string representation of the StringToStringSlice.
func (s *StringToStringSlice) String() string {
	var valuesList []string
	for key, values := range s.value {
		valuesList = append(valuesList, key+"="+strings.Join(values, ","))
	}
	return strings.Join(valuesList, ";")
}

// Strings gets the value of the StringToStringSlice.
func (s *StringToStringSlice) Get() map[string][]string {
	return s.value
}

// Set sets the value of the StringToStringSlice.
func (s *StringToStringSlice) Set(value string) error {
	s.value = make(map[string][]string)

	parts := strings.Split(value, ";")
	for _, part := range parts {
		subparts := strings.Split(part, "=")
		if len(subparts) != 2 {
			return fmt.Errorf("%s not in the form of key1=value1,value2;key2=value1,value2", value)
		}
		key := subparts[0]
		if _, ok := s.value[key]; ok {
			return fmt.Errorf("duplicate key: %s", key)
		}
		values := strings.Split(subparts[1], ",")
		s.value[key] = append(s.value[key], values...)
	}

	return nil
}
