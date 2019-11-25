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

package bugzilla

import "fmt"

// PrettyStatus returns:
//   - "status (resolution)" if both status and resolution are not empty
//   - "status" if only resolution is empty
//   - "any status with resolution RESOLUTION" if only status is empty
//   - "" if both status and resolution are empty
// This is useful in user-facing messages that communicate bug state information
func PrettyStatus(status, resolution string) string {
	if resolution == "" {
		return status
	}
	if status == "" {
		return fmt.Sprintf("any status with resolution %s", resolution)
	}

	return fmt.Sprintf("%s (%s)", status, resolution)
}
