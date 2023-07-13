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

package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	cloudbuild "cloud.google.com/go/cloudbuild/apiv1/v2"
	"cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// GCB automatically assigns random build ids to a new build, so it's not
	// possible to associate a GCB build with a prow job by id without storing
	// mapping somewhere else. Utilizing GCB tags for achieving this.

	// ProwLabelSeparator formats labels key:val pairs into `{key} ::: {val}`
	// The separator was arbitrarily selected.
	ProwLabelSeparator = " ::: "
)

// Operator is the interface that's highly recommended to be used by any package
// that imports current package. Unit test can be carried out with a mock
// implemented under fake of current package.
type Operator interface {
	GetBuild(ctx context.Context, project, id string) (*cloudbuildpb.Build, error)
	ListBuildsByTag(ctx context.Context, project string, tags []string) ([]*cloudbuildpb.Build, error)
	CreateBuild(ctx context.Context, project string, bld *cloudbuildpb.Build) (*cloudbuildpb.Build, error)
	CancelBuild(ctx context.Context, project, id string) (*cloudbuildpb.Build, error)
}

// ProwLabel formats labels key:val pairs into `{key} ::: {val}`.
// These labels will be parsed by prow controller manager for mapping a GCB
// build to a prow job.
func ProwLabel(key, val string) string {
	return key + ProwLabelSeparator + val
}

// KvPairFromProwLabel trims label into key:val pairs. returns empty string as value if
// the label is not formatted as prow label format.
func KvPairFromProwLabel(tag string) (key, val string) {
	parts := strings.SplitN(tag, ProwLabelSeparator, 2)
	if len(parts) != 2 {
		return tag, ""
	}
	return parts[0], parts[1]
}

// GetProwLabels gets labels from cloud build struct, simulating k8s pods labels format
// of map[string]string.
func GetProwLabels(bld *cloudbuildpb.Build) map[string]string {
	res := make(map[string]string)
	for _, tag := range bld.Tags {
		key, val := KvPairFromProwLabel(tag)
		if val != "" {
			res[key] = val
		}
	}
	return res
}

var _ Operator = (*Client)(nil)

// Client wraps native cloudbuild client.
type Client struct {
	interactor *cloudbuild.Client
}

// NewClient creates a new Client, with optional credentialFile.
func NewClient(ctx context.Context, credentialFile string) (*Client, error) {
	var opts []option.ClientOption
	// Authenticating with key file if it's provided.
	if len(credentialFile) > 0 {
		opts = append(opts, option.WithCredentialsFile(credentialFile))
	}

	cbClient, err := cloudbuild.NewClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{interactor: cbClient}, nil
}

// GetBuild gets build by GCB build id.
func (c *Client) GetBuild(ctx context.Context, project, id string) (*cloudbuildpb.Build, error) {
	return c.interactor.GetBuild(ctx, &cloudbuildpb.GetBuildRequest{
		ProjectId: project,
		Id:        id,
	})
}

// ListBuildsByTag lists builds by GCB tags.
//
// This will be used by prow for listing builds triggered by prow, for example
// `created-by-prow ::: true`.
func (c *Client) ListBuildsByTag(ctx context.Context, project string, tags []string) ([]*cloudbuildpb.Build, error) {
	// pageSize is used by ListBuildsByTag only, define here for locality
	// reason. Can move up if pagination is needed by more functions.
	const pageSize = 50
	var res []*cloudbuildpb.Build
	var tagsFilters []string
	for _, tag := range tags {
		tagsFilters = append(tagsFilters, "tags="+tag)
	}

	iter := c.interactor.ListBuilds(ctx, &cloudbuildpb.ListBuildsRequest{
		ProjectId: project,
		PageSize:  pageSize,
		Filter:    strings.Join(tagsFilters, " AND "),
	})
	// ListBuilds already fetches all, just need to do pagination.
	pager := iterator.NewPager(iter, pageSize, "")
	for {
		var buildsInPage []*cloudbuildpb.Build
		nextPageToken, err := pager.NextPage(&buildsInPage)
		if err != nil {
			return nil, err
		}
		if buildsInPage != nil {
			res = append(res, buildsInPage...)
		}
		if nextPageToken == "" {
			break
		}
	}
	return res, nil
}

// CreateBuild creates build and wait for the operation to complete.
func (c *Client) CreateBuild(ctx context.Context, project string, bld *cloudbuildpb.Build) (*cloudbuildpb.Build, error) {
	op, err := c.interactor.CreateBuild(ctx, &cloudbuildpb.CreateBuildRequest{
		ProjectId: project,
		Build:     bld,
	})
	if err != nil {
		return nil, err
	}

	// CreateBuild returns CreateBuildOperation, wait until the operation results in build.
	// CreateBuildOperation contains `Wait` method, which however polls every minute, which is
	// too slow. So use `wait.Poll` instead.
	var triggered *cloudbuildpb.Build
	const (
		pollInterval = 100 * time.Millisecond
		pollTimeout  = 30 * time.Second
	)
	if err := wait.Poll(pollInterval, pollTimeout, func() (bool, error) {
		triggered, err = op.Poll(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to create build in project %s: %w", project, err)
		}
		// op.Poll surprisingly waits until the build completes before returning build,
		// op.Metadata somehow does not wait, so use it instead.
		meta, err := op.Metadata()
		if err != nil {
			return false, fmt.Errorf("failed to get metadata in project %s: %w", project, err)
		}
		triggered = meta.GetBuild()
		return triggered != nil, nil
	}); err != nil {
		return nil, fmt.Errorf("failed waiting for build in project %s appear: %w", project, err)
	}
	return triggered, nil
}

// CancelBuild cancels build and wait for it.
func (c *Client) CancelBuild(ctx context.Context, project, id string) (*cloudbuildpb.Build, error) {
	return c.interactor.CancelBuild(ctx, &cloudbuildpb.CancelBuildRequest{
		ProjectId: project,
		Id:        id,
	})
}
