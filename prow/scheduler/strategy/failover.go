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

// Failover is a scheduling strategy that handles clusters known to be
// in a faulty state. It holds a list of mapping from a broken cluster to an
// healthy one. This strategy get the cluster from a ProwJob and replaces it
// with another one if it was found on the mapping list.
type Failover struct {
	cfg config.FailoverScheduling
}

var _ Interface = &Failover{}

func (f *Failover) Schedule(_ context.Context, pj *prowv1.ProwJob) (Result, error) {
	if cluster, exists := f.cfg.ClusterMappings[pj.Spec.Cluster]; exists {
		return Result{Cluster: cluster}, nil
	}
	return Result{Cluster: pj.Spec.Cluster}, nil
}

func NewFailover(cfg config.FailoverScheduling) *Failover {
	return &Failover{cfg: cfg}
}
