/*
Copyright 2020 The Kubernetes Authors.

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

package util

import (
	"errors"
	"fmt"
	"strings"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
)

func GetJobDestination(cfg config.Getter, pj *prowv1.ProwJob) (bucket, dir string, err error) {
	// We can't divine a destination for jobs that don't have a build ID, so don't try.
	gc, err := gcsConfig(cfg, pj)
	if err != nil {
		return "", "", err
	}
	ps := downwardapi.NewJobSpec(pj.Spec, pj.Status.BuildID, pj.Name)
	_, d, _ := gcsupload.PathsForJob(gc, &ps, "")

	return gc.Bucket, d, nil
}

func IsGCSDestination(cfg config.Getter, pj *prowv1.ProwJob) bool {
	gc, err := gcsConfig(cfg, pj)
	if err != nil || gc.Bucket == "" {
		return false
	}
	if strings.HasPrefix(gc.Bucket, "gs://") {
		return true
	}
	// GCS is default if no other storage type is specified.
	return !strings.Contains(gc.Bucket, "://")
}

func gcsConfig(cfg config.Getter, pj *prowv1.ProwJob) (*prowv1.GCSConfiguration, error) {
	if pj.Status.BuildID == "" {
		return nil, errors.New("cannot get job destination for job with no BuildID")
	}

	if pj.Spec.DecorationConfig != nil && pj.Spec.DecorationConfig.GCSConfiguration != nil {
		return pj.Spec.DecorationConfig.GCSConfiguration, nil
	}

	// The decoration config is always provided for decorated jobs, but many
	// jobs are not decorated, so we guess that we should use the default location
	// for those jobs. This assumption is usually (but not always) correct.
	// The TestGrid configurator uses the same assumption.
	repo := ""
	if pj.Spec.Refs != nil {
		repo = pj.Spec.Refs.Org + "/" + pj.Spec.Refs.Repo
	} else if len(pj.Spec.ExtraRefs) > 0 {
		repo = fmt.Sprintf("%s/%s", pj.Spec.ExtraRefs[0].Org, pj.Spec.ExtraRefs[0].Repo)
	}

	ddc := cfg().Plank.GuessDefaultDecorationConfig(repo, pj.Spec.Cluster)
	if ddc != nil && ddc.GCSConfiguration != nil {
		return ddc.GCSConfiguration, nil
	}
	return nil, fmt.Errorf("couldn't figure out a GCS config for %q", pj.Spec.Job)
}
