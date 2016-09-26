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

package github

import "strings"

// Parse Link headers, returning a map from Rel to URL.
func parseLinks(h string) map[string]string {
	links := map[string]string{}
	for _, l := range strings.Split(h, ",") {
		url := ""
		rel := ""
		for _, p := range strings.Split(l, ";") {
			p = strings.TrimSpace(p)
			if strings.HasPrefix(p, "<") && strings.HasSuffix(p, ">") {
				url = strings.Trim(p, "<>")
			}
			if i := strings.Index(p, "="); i != -1 {
				if strings.ToLower(p[:i]) == "rel" {
					rel = strings.Trim(p[i+1:], "\"")
				}
			}
		}
		if rel != "" && url != "" {
			links[rel] = url
		}
	}
	return links
}
