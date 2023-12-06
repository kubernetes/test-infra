/*
Copyright 2023 The Kubernetes Authors.

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

package resultstore

import (
	"context"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/resultstore/writer"
)

type Uploader struct {
	client writer.ResultStoreBatchClient
}

func NewUploader(client *Client) *Uploader {
	return &Uploader{
		client: client.UploadClient(),
	}
}

// Upload uploads a completed Prow job's results to ResultStore's API:
// https://github.com/googleapis/googleapis/blob/master/google/devtools/resultstore/v2/resultstore_upload.proto
func (u *Uploader) Upload(ctx context.Context, log *logrus.Entry, p *Payload) error {
	inv, err := p.invocation()
	if err != nil {
		return err
	}
	w, err := writer.New(ctx, log, u.client, inv)
	if err != nil {
		return err
	}
	w.WriteConfiguration(ctx, p.defaultConfiguration())
	w.WriteTarget(ctx, p.overallTarget())
	w.WriteConfiguredTarget(ctx, p.configuredTarget())
	w.WriteAction(ctx, p.overallAction())
	w.Finalize(ctx)
	return nil
}
