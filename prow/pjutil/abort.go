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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	reporter "k8s.io/test-infra/prow/crier/reporters/github"
)

// patchClient a minimalistic prow client required by the aborter
type patchClient interface {
	Patch(ctx context.Context, obj ctrlruntimeclient.Object, patch ctrlruntimeclient.Patch, opts ...ctrlruntimeclient.PatchOption) error
}

// prowClient a minimalistic prow client required by the aborter
type prowClient interface {
	Patch(ctx context.Context, name string, pt ktypes.PatchType, data []byte, o metav1.PatchOptions, subresources ...string) (result *prowapi.ProwJob, err error)
}

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

// TerminateOlderJobs aborts all presubmit jobs from the given list that have a newer version. It does not set
// the prowjob to complete. The responsible agent is expected to react to the aborted state by aborting the actual
// test payload and then setting the ProwJob to completed.
func TerminateOlderJobs(pjc patchClient, log *logrus.Entry, pjs []prowapi.ProwJob) error {
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

func PatchProwjob(ctx context.Context, pjc prowClient, log *logrus.Entry, srcPJ prowapi.ProwJob, destPJ prowapi.ProwJob) (*prowapi.ProwJob, error) {
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

	newPJ, err := pjc.Patch(ctx, srcPJ.Name, ktypes.MergePatchType, patch, metav1.PatchOptions{})
	log.WithFields(ProwJobFields(&destPJ)).Debug("Patched ProwJob.")
	return newPJ, err
}
