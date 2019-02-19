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

package flagutil

import (
	"net/url"
)

// URL represents the value of a flag that accept URLs.
type URL struct {
	*url.URL
}

// NewURL returns a URL instance that defaults to the value of def.
func NewURL(def *url.URL) URL {
	return URL{def}
}

// MustParseURL returns a URL instance that defaults to the value of def. Should only used to parse constants since it will panic on any error
func MustParseURL(def string) URL {
	u, err := url.Parse(def)
	if err != nil {
		panic(err)
	}
	return URL{u}
}

// String returns a string representation of the flag.
func (u URL) String() string {
	if u.URL == nil {
		return ""
	}
	return u.URL.String()
}

// Set records the value passed
func (u URL) Set(value string) (err error) {
	u.URL, err = url.Parse(value)
	return
}
