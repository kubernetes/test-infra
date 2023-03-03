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

package cluster

import (
	"testing"

	"k8s.io/test-infra/kubetest/boskos/common"
)

func TestConfig(t *testing.T) {
	config, err := common.ParseConfig("boskos-resources.yaml")
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}

	if err = common.ValidateConfig(config); err != nil {
		t.Fatalf("invalid config: %v", err)
	}
}
