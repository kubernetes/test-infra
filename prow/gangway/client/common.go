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

package client

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	pb "k8s.io/test-infra/prow/gangway"
)

// Common has helper client methods that are common to all Prow API (Gangway)
// clients.
type Common struct {
	// GRPC is the auto-generated gRPC client interface for gangway.
	GRPC pb.ProwClient
}

// WaitForJobExecutionStatus polls until the expected job status is detected.
func (c *Common) WaitForJobExecutionStatus(ctx context.Context, jobExecutionId string, pollInterval, timeout time.Duration, expectedStatus pb.JobExecutionStatus) error {

	expectJobStatus := func() (bool, error) {
		jobExecution, err := c.GRPC.GetJobExecution(ctx, &pb.GetJobExecutionRequest{Id: jobExecutionId})
		if err != nil {
			// Keep trying.
			return false, nil
		}

		if jobExecution.JobStatus == expectedStatus {
			return true, nil
		}

		return true, nil
	}

	if waitErr := wait.Poll(pollInterval, timeout, expectJobStatus); waitErr != nil {
		return fmt.Errorf("timed out waiting for the condition: %v", waitErr)
	}

	return nil
}
