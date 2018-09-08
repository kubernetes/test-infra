/*
Copyright 2018 The Kubernetes Authors.

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

/*
Helper functions shared by more than one other go files
*/

package str

import (
	"fmt"
)

// PercentStr converts a fraction number to percentage string representation
func PercentStr(f float32) string {
	return fmt.Sprintf("%.1f%%", f*100)
}

//PercentageForTestgrid converts a fraction number to percentage string representation used on TestGrid
func PercentageForTestgrid(f float32) string {
	return fmt.Sprintf("%.1f", f*100)
}

//PercentageForCovbotDelta converts a fraction number to percentage string representation used by
// covbot
func PercentageForCovbotDelta(f float32) string {
	return fmt.Sprintf("%.1f", f*100)
}
