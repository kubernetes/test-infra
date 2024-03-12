/*
Copyright 2024 The Kubernetes Authors.

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

package strategy

import (
	"context"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

// Result is an answer that came out of a scheduling strategy
type Result struct {
	// A candidate cluster, the chosen one
	Cluster string
}

// Interface is an interface over scheduling strategies
type Interface interface {
	Schedule(context.Context, *prowv1.ProwJob) (Result, error)
}

// Get gets a scheduling strategy in accordance to configuration. It defaults
// to Passthrough stategy if none has been configured.
func Get(cfg *config.Config) Interface {
	if cfg.Scheduler.Failover != nil {
		return NewFailover(*cfg.Scheduler.Failover)
	}
	return &Passthrough{}
}

// Passthrough is the backward compatible, transparent scheduling strategy, and in fact
// it pretends a scheduler didn't exist at all. This strategy assumes a cluster has
// been assigned to a ProwJob at the time it was defined.
type Passthrough struct {
}

var _ Interface = &Passthrough{}

func (p *Passthrough) Schedule(_ context.Context, pj *prowv1.ProwJob) (Result, error) {
	return Result{Cluster: pj.Spec.Cluster}, nil
}
