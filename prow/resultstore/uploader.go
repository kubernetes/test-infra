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
	"bytes"
	"context"
	"crypto/sha256"
	"sync"

	"github.com/google/uuid"
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

// onlyTransientError returns err only if it is transient. This ensures the
// caller retries the Upload at a later time only for non-permanent errors.
func onlyTransientError(err error) error {
	if writer.IsPermanentError(err) {
		return nil
	}
	return err
}

// Upload uploads a completed Prow job's results to ResultStore's API:
// https://github.com/googleapis/googleapis/blob/master/google/devtools/resultstore/v2/resultstore_upload.proto
// This function distinguishes between transient and permanent errors; only
// transient errors from ResultStore are returned, in which case the call
// should be retried later.
func (u *Uploader) Upload(ctx context.Context, log *logrus.Entry, p *Payload) error {
	invID, err := p.InvocationID()
	if err != nil {
		log.Errorf("p.InvocationID: %v", err)
	}
	inv, err := p.Invocation()
	if err != nil {
		log.Errorf("p.Invocation: %v", err)
		return nil
	}
	w, err := writer.New(ctx, log, u.client, inv, invID, authToken.From(invID))
	if err != nil {
		return onlyTransientError(err)
	}
	// While the resource proto write methods could theoretically return error,
	// because we presently create fewer than writer.batchSize updates, they
	// are all batched and no I/O occurs until the Finalize() call.
	w.WriteConfiguration(ctx, p.DefaultConfiguration())
	w.WriteTarget(ctx, p.OverallTarget())
	w.WriteConfiguredTarget(ctx, p.ConfiguredTarget())
	w.WriteAction(ctx, p.OverallAction())

	if err := w.Finalize(ctx); err != nil {
		return onlyTransientError(err)
	}
	return nil
}

type tokenGenerator struct {
	mu   sync.Mutex
	seed []byte
}

func (g *tokenGenerator) From(id string) string {
	g.mu.Lock()
	seed := g.seed
	g.mu.Unlock()

	digest := sha256.New()
	digest.Write(seed)
	digest.Write([]byte(id))
	r := bytes.NewReader(digest.Sum(nil))
	// error cannot occur since we read from a 256-bit digest.
	u, _ := uuid.NewRandomFromReader(r)
	return u.String()
}

func (g *tokenGenerator) Reseed(seed string) {
	digest := sha256.New()
	digest.Write([]byte(seed))

	g.mu.Lock()
	g.seed = digest.Sum(nil)
	g.mu.Unlock()
}

var authToken *tokenGenerator

func init() {
	authToken = &tokenGenerator{}
	SeedAuthToken("Avast ye, Matey!")
}

// SeedAuthToken sets the seed for computing AuthenticationToken values for
// ResultStore uploads. This is just one of many layers of protection, but if
// ever needed, a Crier secret string could be used to rule out unintended
// writers from interfering with in-progress uploads.
func SeedAuthToken(seed string) {
	authToken.Reseed(seed)
}
