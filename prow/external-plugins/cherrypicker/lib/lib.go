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

package cherrypicker

import "fmt"

// CreateCherrypickBody creates the body of a cherrypick PR
func CreateCherrypickBody(num int, requestor, note string) string {
	cherryPickBody := fmt.Sprintf("This is an automated cherry-pick of #%d", num)
	if len(requestor) != 0 {
		cherryPickBody = fmt.Sprintf("%s\n\n/assign %s", cherryPickBody, requestor)
	}
	if len(note) != 0 {
		cherryPickBody = fmt.Sprintf("%s\n\n%s", cherryPickBody, note)
	}
	return cherryPickBody
}
