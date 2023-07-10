/*
Copyright 2021 The Kubernetes Authors.

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

package fake

import (
	"context"
	"errors"

	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"k8s.io/apimachinery/pkg/util/sets"
	cloudbuild "k8s.io/test-infra/prow/googlecloudbuild/client"
)

// ensure FakeClient implements cloudbuild.Operator
var _ cloudbuild.Operator = (*FakeClient)(nil)

type FakeClient struct {
	// builds in map{project: map{id: *Build}}
	Builds map[string]map[string]*cloudbuildpb.Build
	Err    error
}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		Builds: make(map[string]map[string]*cloudbuildpb.Build),
	}
}

func (fc *FakeClient) GetBuild(ctx context.Context, project, id string) (*cloudbuildpb.Build, error) {
	if fc.Err != nil {
		return nil, fc.Err
	}
	if bldsInProj, ok := fc.Builds[project]; ok {
		if bld, ok := bldsInProj[id]; ok {
			return bld, nil
		}
	}
	return nil, errors.New("not found")
}

func (fc *FakeClient) ListBuildsByTag(ctx context.Context, project string, tags []string) ([]*cloudbuildpb.Build, error) {
	if fc.Err != nil {
		return nil, fc.Err
	}
	var res []*cloudbuildpb.Build
	bldsInProj, ok := fc.Builds[project]
	if !ok {
		return nil, nil
	}
	for _, bld := range bldsInProj {
		bld := bld
		if sets.New[string](bld.Tags...).HasAll(tags...) {
			res = append(res, bld)
		}
	}
	return res, nil
}

// Figure out whether wait here or wait by caller
func (fc *FakeClient) CreateBuild(ctx context.Context, project string, bld *cloudbuildpb.Build) (*cloudbuildpb.Build, error) {
	if fc.Err != nil {
		return nil, fc.Err
	}
	if len(bld.Id) == 0 {
		return nil, errors.New("build Id cannot be empty")
	}
	if _, ok := fc.Builds[project]; !ok {
		fc.Builds[project] = make(map[string]*cloudbuildpb.Build)
	}
	if _, ok := fc.Builds[project][bld.Id]; ok {
		return nil, errors.New("build already exist")
	}
	// The input is modified here. It's good for now since these fields should not be relied
	// on after the build was created.
	bld.Status = cloudbuildpb.Build_QUEUED
	bld.StartTime = timestamppb.Now()
	fc.Builds[project][bld.Id] = bld
	return nil, nil
}

func (fc *FakeClient) CancelBuild(ctx context.Context, project, id string) (*cloudbuildpb.Build, error) {
	if fc.Err != nil {
		return nil, fc.Err
	}
	bld, err := fc.GetBuild(ctx, project, id)
	if err != nil {
		return nil, err
	}
	bld.FinishTime = timestamppb.Now()
	bld.Status = cloudbuildpb.Build_CANCELLED
	return nil, nil
}
