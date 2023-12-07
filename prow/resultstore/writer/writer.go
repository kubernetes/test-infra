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

package writer

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// Number of UploadRequest messages per batch recommended by the
	// ResultStore maintainers. This is likely not a factor unless
	// this implementation is changed to upload individual tests.
	batchSize = 100
)

var (
	// rpcRetryBackoff returns the Backoff for retrying CreateInvocation
	// and UploadBatch requests to ResultStore.
	rpcRetryBackoff = wait.Backoff{
		Duration: 100 * time.Millisecond,
		Factor:   2,
		Cap:      30 * time.Second,
		Steps:    8,
		Jitter:   0.2,
	}
	// rpcRetryDuration returns the time allowed for all retries of a
	// single CreateInvocation or UploadBatch request to ResultStore.
	rpcRetryDuration = 5 * time.Minute
)

func authorizationToken() string {
	// ResultStore authorization tokens "must be set to a RFC 4122-
	// compliant v3, v4, or v5 UUID."
	return uuid.New().String()
}

func resumeToken() string {
	// ResultStore resume tokens must be unique and be "web safe
	// Base64 encoded bytes."
	return base64.StdEncoding.EncodeToString([]byte(uuid.New().String()))
}

type ResultStoreBatchClient interface {
	CreateInvocation(context.Context, *resultstore.CreateInvocationRequest, ...grpc.CallOption) (*resultstore.Invocation, error)
	UploadBatch(ctx context.Context, in *resultstore.UploadBatchRequest, opts ...grpc.CallOption) (*resultstore.UploadBatchResponse, error)
}

// writer writes results to resultstore using the UpdateBatch API.
type writer struct {
	log         *logrus.Entry
	client      ResultStoreBatchClient
	authToken   string
	resumeToken string
	retInv      *resultstore.Invocation
	updates     []*resultstore.UploadRequest
	finalized   bool
}

// persistentError returns whether the error status code is not
// retryable per the ResultStore maintainers. Any codes not
// listed here should be retried with exponential backoff.
func persistentError(err error) bool {
	status, _ := status.FromError(err)
	switch status.Code() {
	case codes.AlreadyExists:
		return true
	case codes.NotFound:
		return true
	case codes.InvalidArgument:
		return true
	case codes.FailedPrecondition:
		return true
	case codes.Unimplemented:
		return true
	case codes.PermissionDenied:
		return true
	}
	return false
}

func New(ctx context.Context, log *logrus.Entry, client ResultStoreBatchClient, inv *resultstore.Invocation) (*writer, error) {
	invID := inv.Id.InvocationId
	w := &writer{
		log:         log,
		client:      client,
		authToken:   authorizationToken(),
		resumeToken: resumeToken(),
		updates:     []*resultstore.UploadRequest{},
	}
	ctx, cancel := context.WithTimeout(ctx, rpcRetryDuration)
	defer cancel()
	err := wait.ExponentialBackoffWithContext(ctx, rpcRetryBackoff, func() (bool, error) {
		inv, err := w.client.CreateInvocation(ctx, w.createInvocationRequest(invID, inv))
		if err != nil {
			log.Errorf("resultstore.CreateInvocation: %v", err)
			if persistentError(err) {
				// End retries by returning error.
				return false, err
			}
			return false, nil
		}
		w.retInv = inv
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *writer) WriteConfiguration(ctx context.Context, c *resultstore.Configuration) error {
	return w.addUploadRequest(ctx, createConfigurationUploadRequest(c))
}

func (w *writer) WriteTarget(ctx context.Context, t *resultstore.Target) error {
	return w.addUploadRequest(ctx, createTargetUploadRequest(t))
}

func (w *writer) WriteConfiguredTarget(ctx context.Context, ct *resultstore.ConfiguredTarget) error {
	return w.addUploadRequest(ctx, createConfiguredTargetUploadRequest(ct))
}

func (w *writer) WriteAction(ctx context.Context, a *resultstore.Action) error {
	return w.addUploadRequest(ctx, createActionUploadRequest(a))
}

func (w *writer) Finalize(ctx context.Context) error {
	return w.addUploadRequest(ctx, w.finalizeRequest())
}

func (w *writer) createInvocationRequest(invID string, inv *resultstore.Invocation) *resultstore.CreateInvocationRequest {
	return &resultstore.CreateInvocationRequest{
		InvocationId:       invID,
		Invocation:         inv,
		AuthorizationToken: w.authToken,
		InitialResumeToken: w.resumeToken,
	}
}

func (w *writer) addUploadRequest(ctx context.Context, r *resultstore.UploadRequest) error {
	if w.finalized {
		return fmt.Errorf("addUploadRequest after finalized for %v", r)
	}
	if r.UploadOperation == resultstore.UploadRequest_FINALIZE {
		w.finalized = true
	}
	w.updates = append(w.updates, r)
	if !w.finalized && len(w.updates) < batchSize {
		return nil
	}
	return w.flushUpdates(ctx)
}

func (w *writer) flushUpdates(ctx context.Context) error {
	b := w.uploadBatchRequest(w.updates)
	ctx, cancel := context.WithTimeout(ctx, rpcRetryDuration)
	defer cancel()
	return wait.ExponentialBackoffWithContext(ctx, rpcRetryBackoff, func() (bool, error) {
		if _, err := w.client.UploadBatch(ctx, b); err != nil {
			w.log.Errorf("resultstore.UploadBatch: %v", err)
			if persistentError(err) {
				// End retries by returning error.
				return false, err
			}
			return false, nil
		}
		w.updates = []*resultstore.UploadRequest{}
		return true, nil
	})
}

func (w *writer) uploadBatchRequest(reqs []*resultstore.UploadRequest) *resultstore.UploadBatchRequest {
	nextToken := resumeToken()
	req := &resultstore.UploadBatchRequest{
		Parent:             w.retInv.Name,
		ResumeToken:        w.resumeToken,
		NextResumeToken:    nextToken,
		AuthorizationToken: w.authToken,
		UploadRequests:     reqs,
	}
	w.resumeToken = nextToken
	return req
}

func (w *writer) finalizeRequest() *resultstore.UploadRequest {
	return &resultstore.UploadRequest{
		UploadOperation: resultstore.UploadRequest_FINALIZE,
		Resource:        &resultstore.UploadRequest_Invocation{},
	}
}

func createConfigurationUploadRequest(c *resultstore.Configuration) *resultstore.UploadRequest {
	return &resultstore.UploadRequest{
		Id: &resultstore.UploadRequest_Id{
			ConfigurationId: c.Id.ConfigurationId,
		},
		UploadOperation: resultstore.UploadRequest_CREATE,
		Resource: &resultstore.UploadRequest_Configuration{
			Configuration: c,
		},
	}
}

func createTargetUploadRequest(t *resultstore.Target) *resultstore.UploadRequest {
	return &resultstore.UploadRequest{
		Id: &resultstore.UploadRequest_Id{
			TargetId: t.Id.GetTargetId(),
		},
		UploadOperation: resultstore.UploadRequest_CREATE,
		Resource: &resultstore.UploadRequest_Target{
			Target: t,
		},
	}
}

func createConfiguredTargetUploadRequest(ct *resultstore.ConfiguredTarget) *resultstore.UploadRequest {
	return &resultstore.UploadRequest{
		Id: &resultstore.UploadRequest_Id{
			TargetId:        ct.Id.GetTargetId(),
			ConfigurationId: ct.Id.GetConfigurationId(),
		},
		UploadOperation: resultstore.UploadRequest_CREATE,
		Resource: &resultstore.UploadRequest_ConfiguredTarget{
			ConfiguredTarget: ct,
		},
	}
}

func createActionUploadRequest(a *resultstore.Action) *resultstore.UploadRequest {
	return &resultstore.UploadRequest{
		Id: &resultstore.UploadRequest_Id{
			TargetId:        a.Id.GetTargetId(),
			ConfigurationId: a.Id.GetActionId(),
			ActionId:        a.Id.GetActionId(),
		},
		UploadOperation: resultstore.UploadRequest_CREATE,
		Resource: &resultstore.UploadRequest_Action{
			Action: a,
		},
	}
}
