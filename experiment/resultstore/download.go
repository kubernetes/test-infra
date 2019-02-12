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

package main

import (
	"context"
	"fmt"

	"k8s.io/test-infra/testgrid/util/gcs"
)

type downloadResult struct {
	started      gcs.Started
	finished     gcs.Finished
	artifactURLs []string
	suiteMetas   []gcs.SuitesMeta
}

func download(ctx context.Context, opt options) (*downloadResult, error) {
	var creds []string
	if opt.account != "" {
		creds = append(creds, opt.account)
	}
	client, err := gcs.ClientWithCreds(ctx, creds...)
	if err != nil {
		return nil, fmt.Errorf("client: %v", err)
	}

	build := gcs.Build{
		Bucket:     client.Bucket(opt.path.Bucket()),
		Context:    ctx,
		Prefix:     trailingSlash(opt.path.Object()),
		BucketPath: opt.path.Bucket(),
	}

	fmt.Println("Read started...")
	started, err := build.Started()
	if err != nil {
		return nil, fmt.Errorf("started: %v", err)
	}
	fmt.Println("Read finished...")
	finished, err := build.Finished()
	if err != nil {
		return nil, fmt.Errorf("finished: %v", err)
	}

	ec := make(chan error, 2)
	artifacts := make(chan string)
	suitesChan := make(chan gcs.SuitesMeta)
	fmt.Println("List suites...")

	go func() { // err1
		defer close(artifacts)
		err := build.Artifacts(artifacts)
		if err != nil {
			err = fmt.Errorf("artifacts: %v", err)
		}
		ec <- err
	}()

	var artifactURLs []string

	for a := range artifacts {
		artifactURLs = append(artifactURLs, a)
	}

	artifacts = make(chan string)
	go func() {
		defer close(artifacts)
		for _, a := range artifactURLs {
			select {
			case artifacts <- a:
			case <-ctx.Done():
			}
		}
	}()

	go func() { // err2
		defer close(suitesChan)
		err := build.Suites(artifacts, suitesChan)
		if err != nil {
			err = fmt.Errorf("suites: %v", err)
		}
		ec <- err
	}()

	var suites []gcs.SuitesMeta
	for s := range suitesChan {
		suites = append(suites, s)
	}

	err1, err2 := <-ec, <-ec
	if err1 != nil {
		return nil, err1
	}
	if err2 != nil {
		return nil, err2
	}

	return &downloadResult{
		started:      *started,
		finished:     *finished,
		artifactURLs: artifactURLs,
		suiteMetas:   suites,
	}, nil
}
