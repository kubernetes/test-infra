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

package flagutil

import (
	"flag"
	"fmt"
	"time"
)

type ControllerManagerOptions struct {
	TimeoutListingProwJobs        time.Duration
	TimeoutListingProwJobsDefault time.Duration
}

func (o *ControllerManagerOptions) AddFlags(fs *flag.FlagSet) {
	fs.DurationVar(&o.TimeoutListingProwJobs, "timeout-listing-prowjobs", o.TimeoutListingProwJobsDefault, "Timeout for listing prowjobs.")
}

func (o *ControllerManagerOptions) Validate(_ bool) error {

	if o.TimeoutListingProwJobsDefault == 0 {
		return fmt.Errorf("programmer error: TimeoutListingProwJobsDefault cannot be 0; please set it before calling AddFlags() for ControllerManagerOptions")
	}

	return nil
}
