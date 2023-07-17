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

import "flag"

type GerritOptions struct {
	GerritFields
	defaults GerritFields
}

type GerritFields struct {
	MaxQPS, MaxBurst int // No throttling when unset.
}

func (o *GerritOptions) SetDefaultThrottle(MaxQPS, MaxBurst int) {
	o.defaults.MaxQPS = MaxQPS
	o.defaults.MaxBurst = MaxBurst
}

func (o *GerritOptions) AddFlags(fs *flag.FlagSet) {
	fs.IntVar(&o.MaxQPS, "gerrit-max-qps", o.defaults.MaxQPS, "The maximum allowed queries per second to the Gerrit API from this component.")
	fs.IntVar(&o.MaxBurst, "gerrit-max-burst", o.defaults.MaxBurst, "The maximum allowed burst size of queries to the Gerrit API from this component.")
}

func (o *GerritOptions) Validate(dryrun bool) error {
	return nil
}
