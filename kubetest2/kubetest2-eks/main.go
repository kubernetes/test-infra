/*
Copyright 2020 The Kubernetes Authors.

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
	"github.com/aws/aws-k8s-tester/eks"
	"github.com/aws/aws-k8s-tester/eksconfig"
	"github.com/spf13/pflag"
	"k8s.io/test-infra/kubetest2/pkg/app"
	"k8s.io/test-infra/kubetest2/pkg/types"

	// import the standard set of testers so they are loaded & registered
	_ "k8s.io/test-infra/kubetest2/pkg/app/testers/standard"
)

func main() {
	app.Main("eks", newEKS)
}

// "opts" is not used in eks deployer, handled via "kubetest2/pkg/app"
// eks uses env vars, so just return empty flag set
func newEKS(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	cfg := eksconfig.NewDefault() // use auto-generated config

	err := cfg.UpdateFromEnvs()
	if err != nil {
		panic(err)
	}

	if err = cfg.ValidateAndSetDefaults(); err != nil {
		panic(err)
	}

	dp, err := eks.New(cfg)
	if err != nil {
		panic(err)
	}

	return dp, pflag.NewFlagSet("eks", pflag.ContinueOnError)
}
