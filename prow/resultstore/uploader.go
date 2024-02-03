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
// This function distinguishes between transient and permanent errors; only
// transient errors from ResultStore are returned, in which case the call
// should be retried later.
func (u *Uploader) Upload(ctx context.Context, log *logrus.Entry, p *Payload) error {
	inv, err := p.Invocation()
	if err != nil {
		log.Errorf("p.Invocation: %v", err)
		return nil
	}
	w, err := writer.New(ctx, log, u.client, inv)
	if err != nil {
		if writer.PermanentError(err) {
			return nil
		}
		return err
	}
	// While the resource proto write methods could theoretically return error,
	// because we presently create fewer than writer.batchSize updates, they
	// are all batched and no I/O occurs until the Finalize() call.
	w.WriteConfiguration(ctx, p.DefaultConfiguration())
	w.WriteTarget(ctx, p.OverallTarget())
	w.WriteConfiguredTarget(ctx, p.ConfiguredTarget())
	w.WriteAction(ctx, p.OverallAction())
	// TODO: When writer.New() is emended to resume uploads (see TODO there),
	// return non-permanent errors from Finalize(). Doing this now is futile,
	// since attempting to upload an existing invocation is a permanent error.
	w.Finalize(ctx)
	return nil
}
