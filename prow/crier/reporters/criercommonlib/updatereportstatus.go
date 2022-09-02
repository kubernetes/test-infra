/*
Copyright 2022 The Kubernetes Authors.

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

// Package criercommonlib contains shared lib used by reporters
package criercommonlib

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func updateReportState(ctx context.Context, pj *prowv1.ProwJob, log *logrus.Entry, reportedState prowv1.ProwJobState, pjclientset ctrlruntimeclient.Client, reporterName string) error {
	// update pj report status
	newpj := pj.DeepCopy()
	// we set omitempty on PrevReportStates, so here we need to init it if is nil
	if newpj.Status.PrevReportStates == nil {
		newpj.Status.PrevReportStates = map[string]prowv1.ProwJobState{}
	}
	newpj.Status.PrevReportStates[reporterName] = reportedState

	if err := pjclientset.Patch(ctx, newpj, ctrlruntimeclient.MergeFrom(pj)); err != nil {
		return fmt.Errorf("failed to patch: %w", err)
	}

	// Block until the update is in the lister to make sure that events from another controller
	// that also does reporting dont trigger another report because our lister doesn't yet contain
	// the updated Status
	name := types.NamespacedName{Namespace: pj.Namespace, Name: pj.Name}
	if err := wait.Poll(100*time.Millisecond, 10*time.Second, func() (bool, error) {
		if err := pjclientset.Get(ctx, name, pj); err != nil {
			return false, err
		}
		if pj.Status.PrevReportStates != nil &&
			pj.Status.PrevReportStates[reporterName] == reportedState {
			return true, nil
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("failed to wait for updated report status to be in lister: %w", err)
	}
	return nil
}

func UpdateReportStateWithRetries(ctx context.Context, pj *prowv1.ProwJob, log *logrus.Entry, pjclientset ctrlruntimeclient.Client, reporterName string) error {
	reportState := pj.Status.State
	log = log.WithFields(logrus.Fields{
		"prowjob":   pj.Name,
		"jobName":   pj.Spec.Job,
		"jobStatus": reportState,
	})
	// We have to retry here, if we return we lose the information that we already reported this job.
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get it first, this is very cheap
		name := types.NamespacedName{Namespace: pj.Namespace, Name: pj.Name}
		if err := pjclientset.Get(ctx, name, pj); err != nil {
			return err
		}
		// Must not wrap until we have kube 1.19, otherwise the RetryOnConflict won't recognize conflicts
		// correctly
		return updateReportState(ctx, pj, log, reportState, pjclientset, reporterName)
	}); err != nil {
		// Very subpar, we will report again. But even if we didn't do that now, we would do so
		// latest when crier gets restarted. In an ideal world, all reporters are idempotent and
		// reporting has no cost.
		return fmt.Errorf("failed to update report state on prowjob: %w", err)
	}

	log.Info("Successfully updated report state on prowjob")
	return nil
}
