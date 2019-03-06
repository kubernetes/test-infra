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

// Resultstore converts --build=gs://prefix/JOB/NUMBER from prow's pod-tuils to a ResultStore invocation suite, which it optionally will --upload=gcp-project.
package main

import (
	"context"
	"fmt"

	"k8s.io/test-infra/testgrid/resultstore"
)

func resultstoreClient(ctx context.Context, account string, secret resultstore.Secret) (*resultstore.Client, error) {
	conn, err := resultstore.Connect(ctx, account)
	if err != nil {
		return nil, err
	}
	rsClient := resultstore.NewClient(conn).WithContext(ctx)
	if secret != "" {
		rsClient = rsClient.WithSecret(secret)
	} else {
		secret := resultstore.NewSecret()
		fmt.Println("Secret:", secret)
		rsClient = rsClient.WithSecret(secret)
	}
	return rsClient, nil
}

// upload the result downloaded from path into project.
func upload(rsClient *resultstore.Client, inv resultstore.Invocation, target resultstore.Target, test resultstore.Test) (string, error) {

	targetID := test.Name
	const configID = resultstore.Default
	invName, err := rsClient.Invocations().Create(inv)
	if err != nil {
		return "", fmt.Errorf("create invocation: %v", err)
	}
	targetName, err := rsClient.Targets(invName).Create(targetID, target)
	if err != nil {
		return resultstore.URL(invName), fmt.Errorf("create target: %v", err)
	}
	url := resultstore.URL(targetName)
	_, err = rsClient.Configurations(invName).Create(configID)
	if err != nil {
		return url, fmt.Errorf("create configuration: %v", err)
	}
	ctName, err := rsClient.ConfiguredTargets(targetName, configID).Create(test.Action)
	if err != nil {
		return url, fmt.Errorf("create configured target: %v", err)
	}
	_, err = rsClient.Actions(ctName).Create("primary", test)
	if err != nil {
		return url, fmt.Errorf("create action: %v", err)
	}
	return url, nil
}
