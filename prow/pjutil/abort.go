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

// jobIdentifier keeps the information required to uniquely identify a prow job
type jobIdentifier struct {
	job          string
	organization string
	repository   string
	pullRequest  int
}

// String returns the string representation of a prow job identifier
func (i *jobIdentifier) String() string {
	return fmt.Sprintf("%s %s/%s#%d", i.job, i.organization, i.repository, i.pullRequest)
}

// olderPresubmits filters the list down to those that have a newer version of the same job.
func olderPresubmits(pjs []prowapi.ProwJob) []prowapi.ProwJob {
	var dupes []prowapi.ProwJob
	found := map[jobIdentifier]prowapi.ProwJob{}
	for _, pj := range pjs {
		if pj.Complete() || pj.Spec.Type != prowapi.PresubmitJob {
			continue
		}
		ident := jobIdentifier{
			job:          pj.Spec.Job,
			organization: pj.Spec.Refs.Org,
			repository:   pj.Spec.Refs.Repo,
			pullRequest:  pj.Spec.Refs.Pulls[0].Number,
		}
		prev, ok := found[ident]
		if !ok {
			found[ident] = pj
			continue
		}
		if pj.Status.StartTime.Before(&prev.Status.StartTime) {
			dupes = append(dupes, pj)
			continue
		}
		found[ident] = pj
		dupes = append(dupes, prev)
	}
	return dupes
}

type Updater func(prowapi.ProwJob) error

func abort(pj prowapi.ProwJob, update, cleanup Updater) error {
	pj.SetComplete()
	pj.Status.State = prowapi.AbortedState
	if err := update(pj); err != nil {
		return fmt.Errorf("update: %v", err)
	}
	if err := cleanup(pj); err != nil {
		return fmt.Errorf("cleanup: %v", err)
	}
	return nil
}

// AbortDuplicates aborts all presubmit jobs from the given list that have a newer version.
func AbortDuplicates(pjs []prowapi.ProwJob, parent *logrus.Entry, update, cleanup Updater) error {
	var errs []error
	dupes := olderPresubmits(pjs)
	for _, pj := range dupes {
		log := parent.WithFields(ProwJobFields(&pj)).WithField("state", pj.Status.State)
		log.Debug("Aborting")
		if err := abort(pj, update, cleanup); err != nil {
			log.WithError(err).Warning("Error aborting duplicate job")
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%d errors aborting %d jobs: %v", len(errs), len(dupes), errs)

}
