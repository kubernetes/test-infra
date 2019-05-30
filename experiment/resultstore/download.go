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

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/testgrid/util/gcs"
)

type downloadResult struct {
	started      gcs.Started
	finished     gcs.Finished
	artifactURLs []string
	suiteMetas   []gcs.SuitesMeta
}

func storageClient(ctx context.Context, account string) (*storage.Client, error) {
	var creds []string
	if account != "" {
		creds = append(creds, account)
	}
	return gcs.ClientWithCreds(ctx, creds...)
}

func download(ctx context.Context, client *storage.Client, build gcs.Build) (*downloadResult, error) {

	log := logrus.WithFields(logrus.Fields{"build": build})
	log.Debug("Read started...")
	started, err := build.Started()
	if err != nil {
		return nil, fmt.Errorf("started: %v", err)
	}
	log.Debug("Read finished...")
	finished, err := build.Finished()
	if err != nil {
		return nil, fmt.Errorf("finished: %v", err)
	}

	ec := make(chan error, 2)
	artifacts := make(chan string)
	suitesChan := make(chan gcs.SuitesMeta)
	log.Debug("List suites...")
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
