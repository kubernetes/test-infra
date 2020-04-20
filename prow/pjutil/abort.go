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
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	reporter "k8s.io/test-infra/prow/crier/reporters/github"
)

// patchClient a minimalistic prow client required by the aborter
type patchClient interface {
	Patch(ctx context.Context, obj runtime.Object, patch ctrlruntimeclient.Patch, opts ...ctrlruntimeclient.PatchOption) error
}

// prowClient a minimalistic prow client required by the aborter
type prowClient interface {
	Patch(name string, pt ktypes.PatchType, data []byte, subresources ...string) (result *prowapi.ProwJob, err error)
}

// ProwJobResourcesCleanup type for a callback function which it is expected to clean up
// all k8s resources associated with the given prow job. It should do the best effort to
// remove these resources, but if for any reason there is an error, it should only log a warning
// message.
type ProwJobResourcesCleanup func(pj prowapi.ProwJob) error

// digestRefs digests a Refs to the fields we care about
// for termination, ensuring that permutations of pulls
// do not cause different digests
func digestRefs(ref prowapi.Refs) string {
	var pulls []int
	for _, pull := range ref.Pulls {
		pulls = append(pulls, pull.Number)
	}
	sort.Ints(pulls)
	return fmt.Sprintf("%s/%s@%s %v", ref.Org, ref.Repo, ref.BaseRef, pulls)
}

// TerminateOlderJobs aborts all presubmit jobs from the given list that have a newer version. It calls
// the cleanup callback for each job before updating its status as aborted.
func TerminateOlderJobs(pjc patchClient, log *logrus.Entry, pjs []prowapi.ProwJob,
	cleanup ProwJobResourcesCleanup) error {
	dupes := map[string]int{}
	for i, pj := range pjs {
		if pj.Complete() || pj.Spec.Type != prowapi.PresubmitJob {
			continue
		}

		// we want to use salient fields of the job spec to create
		// an identifier, so we digest the job spec and to ensure
		// reentrancy, we must sort all of the slices in our identifier
		// so that equivalent permutations of the refs map to the
		// same identifier. We do not want commit hashes to matter
		// here as a test for a newer set of commits but for the
		// same set of names can abort older versions. We digest
		// into strings as Go doesn't define equality for slices,
		// so they are not valid to use in map keys.
		identifiers := []string{
			string(pj.Spec.Type),
			pj.Spec.Job,
		}
		if pj.Spec.Refs != nil {
			identifiers = append(identifiers, digestRefs(*pj.Spec.Refs))
		}
		for _, ref := range pj.Spec.ExtraRefs {
			identifiers = append(identifiers, digestRefs(ref))
		}

		sort.Strings(identifiers)
		ji := strings.Join(identifiers, ",")
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
		prevPJ := toCancel.DeepCopy()

		// TODO cancel the prow job before cleaning up its resources and make this system
		// independent.
		// See this discussion for more details:  https://github.com/kubernetes/test-infra/pull/11451#discussion_r263523932
		if err := cleanup(toCancel); err != nil {
			log.WithError(err).WithFields(ProwJobFields(&toCancel)).Warn("Cannot clean up job resources")
		}

		toCancel.SetComplete()
		toCancel.Status.State = prowapi.AbortedState
		if toCancel.Status.PrevReportStates == nil {
			toCancel.Status.PrevReportStates = map[string]prowapi.ProwJobState{}
		}
		toCancel.Status.PrevReportStates[reporter.GitHubReporterName] = toCancel.Status.State

		log.WithFields(ProwJobFields(&toCancel)).
			WithField("from", prevPJ.Status.State).
			WithField("to", toCancel.Status.State).Info("Transitioning states")

		if err := pjc.Patch(context.Background(), &toCancel, ctrlruntimeclient.MergeFrom(prevPJ)); err != nil {
			return err
		}

		// Update the cancelled jobs entry in pjs.
		pjs[cancelIndex] = toCancel
	}

	return nil
}

func PatchProwjob(pjc prowClient, log *logrus.Entry, srcPJ prowapi.ProwJob, destPJ prowapi.ProwJob) (*prowapi.ProwJob, error) {
	srcPJData, err := json.Marshal(srcPJ)
	if err != nil {
		return nil, fmt.Errorf("marshal source prow job: %v", err)
	}

	destPJData, err := json.Marshal(destPJ)
	if err != nil {
		return nil, fmt.Errorf("marshal dest prow job: %v", err)
	}

	patch, err := jsonpatch.CreateMergePatch(srcPJData, destPJData)
	if err != nil {
		return nil, fmt.Errorf("cannot create JSON patch: %v", err)
	}

	newPJ, err := pjc.Patch(srcPJ.Name, ktypes.MergePatchType, patch)
	log.WithFields(ProwJobFields(&destPJ)).Debug("Patched ProwJob.")
	return newPJ, err
}
