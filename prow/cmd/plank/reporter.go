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
	"context"
	"fmt"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowjobinformer "k8s.io/test-infra/prow/client/informers/externalversions"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/crier"
	githubreporter "k8s.io/test-infra/prow/crier/reporters/github"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/kube"
)

func deprecatedReporter(github flagutil.GitHubOptions, kubernetes flagutil.KubernetesOptions, dryRun bool, cfg config.Getter) (func(context.Context), error) {
	var secretAgent secret.Agent
	if github.TokenPath != "" {
		if err := secretAgent.Start([]string{github.TokenPath}); err != nil {
			return nil, fmt.Errorf("start secret agent: %w", err)
		}
	}

	githubClient, err := github.GitHubClient(&secretAgent, dryRun)
	if err != nil {
		return nil, fmt.Errorf("github client: %w", err)
	}

	prowjobClientset, err := kubernetes.ProwJobClientset(cfg().ProwJobNamespace, dryRun)
	if err != nil {
		return nil, fmt.Errorf("prow client: %w", err)
	}

	const resync = 0
	prowjobInformerFactory := prowjobinformer.NewSharedInformerFactoryWithOptions(prowjobClientset, resync, prowjobinformer.WithNamespace(cfg().ProwJobNamespace))
	informer := prowjobInformerFactory.Prow().V1().ProwJobs()
	githubReporter := githubreporter.NewReporter(githubClient, cfg, prowapi.ProwJobAgent(""))
	controller := crier.NewController(
		prowjobClientset,
		kube.RateLimiter(githubReporter.GetName()),
		informer,
		githubReporter,
		1)
	return controller.Run, nil
}
