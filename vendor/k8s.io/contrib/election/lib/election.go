/*
Copyright 2015 The Kubernetes Authors.

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

package lib

import (
	"encoding/json"
	"os"
	"time"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/client/leaderelection"
	"k8s.io/kubernetes/pkg/client/record"
	client "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/util/wait"
)

const (
	startBackoff = time.Second
	maxBackoff   = time.Minute
)

func getCurrentLeader(electionId, namespace string, c client.Interface) (string, *api.Endpoints, error) {
	endpoints, err := c.Endpoints(namespace).Get(electionId)
	if err != nil {
		return "", nil, err
	}
	val, found := endpoints.Annotations[leaderelection.LeaderElectionRecordAnnotationKey]
	if !found {
		return "", endpoints, nil
	}
	electionRecord := leaderelection.LeaderElectionRecord{}
	if err := json.Unmarshal([]byte(val), &electionRecord); err != nil {
		return "", nil, err
	}
	return electionRecord.HolderIdentity, endpoints, err
}

// NewSimpleElection creates an election, it defaults namespace to 'default' and ttl to 10s
func NewSimpleElection(electionId, id string, callback func(leader string), c client.Interface) (*leaderelection.LeaderElector, error) {
	return NewElection(electionId, id, api.NamespaceDefault, 10*time.Second, callback, c)
}

// NewElection creates an election.  'namespace'/'election' should be an existing Kubernetes Service
// 'id' is the id if this leader, should be unique.
func NewElection(electionId, id, namespace string, ttl time.Duration, callback func(leader string), c client.Interface) (*leaderelection.LeaderElector, error) {
	_, err := c.Endpoints(namespace).Get(electionId)
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = c.Endpoints(namespace).Create(&api.Endpoints{
				ObjectMeta: api.ObjectMeta{
					Name: electionId,
				},
			})
			if err != nil && !errors.IsConflict(err) {
				return nil, err
			}
		} else {
			return nil, err
		}
	}

	leader, endpoints, err := getCurrentLeader(electionId, namespace, c)
	if err != nil {
		return nil, err
	}
	callback(leader)

	broadcaster := record.NewBroadcaster()
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	recorder := broadcaster.NewRecorder(api.EventSource{
		Component: "leader-elector",
		Host:      hostname,
	})

	callbacks := leaderelection.LeaderCallbacks{
		OnStartedLeading: func(stop <-chan struct{}) {
			callback(id)
		},
		OnStoppedLeading: func() {
			leader, _, err := getCurrentLeader(electionId, namespace, c)
			if err != nil {
				glog.Errorf("failed to get leader: %v", err)
				// empty string means leader is unknown
				callback("")
				return
			}
			callback(leader)
		},
	}

	config := leaderelection.LeaderElectionConfig{
		Client:        c,
		EventRecorder: recorder,
		EndpointsMeta: endpoints.ObjectMeta,
		Identity:      id,
		LeaseDuration: ttl,
		RenewDeadline: ttl / 2,
		RetryPeriod:   ttl / 4,
		Callbacks:     callbacks,
	}

	return leaderelection.NewLeaderElector(config)
}

// RunElection runs an election given an leader elector.  Doesn't return.
func RunElection(e *leaderelection.LeaderElector) {
	wait.Forever(e.Run, 0)
}
