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

package clientutil

import (
	"fmt"
	"strings"
)

// HostsFlag is the flag type for slack|rocketchat hosts while initializing slack|rocketchat client
type HostsFlag map[string]string

func (h *HostsFlag) String() string {
	var hosts []string
	for host, tokenPath := range *h {
		hosts = append(hosts, host+"="+tokenPath)
	}
	return strings.Join(hosts, " ")
}

// Set populates ProjectsFlag upon flag.Parse()
func (h *HostsFlag) Set(value string) error {
	if len(*h) == 0 {
		*h = map[string]string{}
	}
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("%s not in the form of host=token-path", value)
	}
	host, tokenPath := parts[0], parts[1]
	if _, ok := (*h)[host]; ok {
		return fmt.Errorf("duplicate host: %s", host)
	}
	(*h)[host] = tokenPath
	return nil
}

// Logger provides an interface to log debug messages.
type Logger interface {
	Debugf(s string, v ...interface{})
}
