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

package pjutil

import (
	"fmt"

	"github.com/sirupsen/logrus"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

// prowClient a minimalistic prow client required by the aborter
type prowClient interface {
	//ReplaceProwJob replaces the prow job with the given name
	ReplaceProwJob(string, prowapi.ProwJob) (prowapi.ProwJob, error)
}

// ProwJobResourcesCleanup type for a callback function which it is expected to clean up
// all k8s resources associated with the given prow job. It should do the best effort to
// remove these resources, but if for any reason there is an error, it should only log a warning
// message.
type ProwJobResourcesCleanup func(pj prowapi.ProwJob) error

// jobIndentifier keeps the information required to uniquely identify a prow job
type jobIndentifier struct {
	job          string
	organization string
	repository   string
	pullRequest  int
}

// String returns the string representation of a prow job identifier
func (i *jobIndentifier) String() string {
	return fmt.Sprintf("%s %s/%s#%d", i.job, i.organization, i.repository, i.pullRequest)
}

// TerminateOlderPresubmitJobs aborts all presubmit jobs from the given list that have a newer version. It calls
// the cleanup callback for each job before updating its status as aborted.
func TerminateOlderPresubmitJobs(pjc prowClient, log *logrus.Entry, pjs []prowapi.ProwJob,
	cleanup ProwJobResourcesCleanup) error {
	dupes := map[jobIndentifier]int{}
	for i, pj := range pjs {
		if pj.Complete() || pj.Spec.Type != prowapi.PresubmitJob {
			continue
		}

		ji := jobIndentifier{
			job:          pj.Spec.Job,
			organization: pj.Spec.Refs.Org,
			repository:   pj.Spec.Refs.Repo,
			pullRequest:  pj.Spec.Refs.Pulls[0].Number,
		}
		prev, ok := dupes[ji]
		if !ok {
			dupes[ji] = i
			continue
		}
		cancelIndex := i
		if (&pjs[prev].Status.StartTime).Before(&pj.Status.StartTime) {
			cancelIndex = prev
			dupes[ji] = i
		}
		toCancel := pjs[cancelIndex]

		// TODO cancel the prow job before cleaning up its resources and make this system
		// independent.
		// See this discussion for more details:  https://github.com/kubernetes/test-infra/pull/11451#discussion_r263523932
		if err := cleanup(toCancel); err != nil {
			log.WithError(err).WithFields(ProwJobFields(&toCancel)).Warn("Cannot clean up job resources")
		}

		toCancel.SetComplete()
		prevState := toCancel.Status.State
		toCancel.Status.State = prowapi.AbortedState
		log.WithFields(ProwJobFields(&toCancel)).
			WithField("from", prevState).
			WithField("to", toCancel.Status.State).Info("Transitioning states")

		npj, err := pjc.ReplaceProwJob(toCancel.ObjectMeta.Name, toCancel)
		if err != nil {
			return err
		}
		pjs[cancelIndex] = npj
	}

	return nil
}
