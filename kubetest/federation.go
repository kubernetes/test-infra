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
	"strings"
)

func FedUp() error {
	return finishRunning(exec.Command("./federation/cluster/federation-up.sh"))
}

func FederationTest(testArgs, dump string) error {
	f := strings.Fields(testArgs)
	f = setReportDir(f, dump)
	f = setFieldDefault(f, "--ginkgo.focus", "\\[Feature:Federation\\]")
	return finishRunning(exec.Command("./hack/federated-ginkgo-e2e.sh", f...))
}

func FedDown() error {
	return finishRunning(exec.Command("./federation/cluster/federation-down.sh"))
}
