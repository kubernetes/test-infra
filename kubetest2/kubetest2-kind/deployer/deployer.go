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

// Package deployer implements the kubetest2 kind deployer
package deployer

import (
	"k8s.io/test-infra/kubetest2/pkg/types"
)

// Name is the name of the deployer
const Name = "kind"

// New implements deployer.New for kind
func New(common types.Options, deployerArgs []string) (types.Deployer, error) {
	// TODO(bentheelder): process arguments for more options
	return &deployer{
		commonOptions: common,
	}, nil
}

// assert that New implements types.NewDeployer
var _ types.NewDeployer = New

// TODO(bentheelder): finish implementing this stubbed-out deployer
type deployer struct {
	commonOptions types.Options
}

// assert that deployer implements types.Deployer
var _ types.Deployer = &deployer{}

func (d *deployer) Up() error {
	panic("unimplemented")
}

func (d *deployer) Down() error {
	panic("unimplemented")
}

func (d *deployer) IsUp() (up bool, err error) {
	panic("unimplemented")
}

func (d *deployer) DumpClusterLogs() error {
	panic("unimplemented")
}

func (d *deployer) Build() error {
	panic("unimplemented")
}
