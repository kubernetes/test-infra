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
	"os/exec"
)

type bash struct{}

func (b bash) Up() error {
	return finishRunning(exec.Command("./hack/e2e-internal/e2e-up.sh"))
}

func (b bash) IsUp() error {
	return finishRunning(exec.Command("./hack/e2e-internal/e2e-status.sh"))
}

func (b bash) SetupKubecfg() error {
	return nil
}

func (b bash) Down() error {
	return finishRunning(exec.Command("./hack/e2e-internal/e2e-down.sh"))
}
