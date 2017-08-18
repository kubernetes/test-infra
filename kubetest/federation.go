/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"errors"
	"os/exec"
	"strings"
)

/*
 multiClusterDeployment type holds the data passed to `--multi-clusters` flag.
 The format of value that should be passed to the flag is `[Zone1:]Cluster1[,[ZoneN:]ClusterN]]*`.
 Multiple clusters can be specified as a comma separated list.
 Zone can be optionally specified along with cluster name as described above in the format.
 If zone is not specified along with cluster then cluster would be deployed in default zone.
*/
type multiClusterDeployment struct {
	zones    map[string]string
	clusters []string
}

func (m *multiClusterDeployment) String() string {
	var str string
	for _, cluster := range m.clusters {
		if len(str) != 0 {
			str += ","
		}
		zone, exist := m.zones[cluster]
		if exist {
			str += zone + ":"
		}
		str += cluster
	}
	return str
}

func (m *multiClusterDeployment) Set(value string) error {
	if len(value) == 0 {
		return errors.New("invalid value passed to --multi-clusters flag, should specify at least one cluster")
	}

	if m.zones == nil {
		m.zones = make(map[string]string)
	}
	clusterZones := strings.Split(value, ",")
	for _, czTuple := range clusterZones {
		czSlice := strings.SplitN(czTuple, ":", 2)
		if len(czSlice[0]) == 0 || (len(czSlice) == 2 && len(czSlice[1]) == 0) {
			return errors.New("invalid value passed to --multi-clusters flag")
		}
		if len(czSlice) == 2 {
			m.zones[czSlice[1]] = czSlice[0]
			m.clusters = append(m.clusters, czSlice[1])
		} else {
			m.clusters = append(m.clusters, czSlice[0])
		}
	}
	return nil
}

func (m *multiClusterDeployment) Enabled() bool {
	return len(m.clusters) > 0
}

func fedUp() error {
	return finishRunning(exec.Command("./federation/cluster/federation-up.sh"))
}

func federationTest(testArgs []string) error {
	testArgs = setFieldDefault(testArgs, "--ginkgo.focus", "\\[Feature:Federation\\]")
	return finishRunning(exec.Command("./hack/federated-ginkgo-e2e.sh", testArgs...))
}

func fedDown() error {
	return finishRunning(exec.Command("./federation/cluster/federation-down.sh"))
}
