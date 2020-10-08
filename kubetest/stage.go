/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"k8s.io/release/pkg/build"
	"k8s.io/release/pkg/object"

	"k8s.io/test-infra/kubetest/util"
)

type stageStrategy struct {
	bucket         string
	ci             bool
	gcsSuffix      string
	versionSuffix  string
	dockerRegistry string
	noAllowDup     bool
	useKrel        bool
}

// Return something like gs://bucket/ci/suffix
func (s *stageStrategy) String() string {
	return fmt.Sprintf("%v%v%v", s.bucket, s.releaseType(), s.gcsSuffix)
}

func (s *stageStrategy) releaseType() string {
	if s.ci {
		return "ci"
	}
	return "devel"
}

// Parse bucket, ci, suffix from gs://BUCKET/ci/SUFFIX
func (s *stageStrategy) Set(value string) error {
	re := regexp.MustCompile(`^(gs://[\w-]+)/(devel|ci)(/.*)?`)
	mat := re.FindStringSubmatch(value)
	if mat == nil {
		return fmt.Errorf("Invalid stage location: %v. Use gs://bucket/ci/optional-suffix", value)
	}
	s.bucket = mat[1]
	s.ci = mat[2] == "ci"
	s.gcsSuffix = mat[3]
	return nil
}

func (s *stageStrategy) Type() string {
	return "stageStrategy"
}

// True when this kubetest invocation wants to stage the release
func (s *stageStrategy) Enabled() bool {
	return s.bucket != ""
}

// Stage the release build to GCS.
func (s *stageStrategy) Stage() error {
	// Trim the gcs prefix from the bucket
	s.bucket = strings.TrimPrefix(s.bucket, object.GcsPrefix)

	// Essentially release/push-build.sh --bucket=B --ci? --gcs-suffix=S
	// --noupdatelatest
	if !s.useKrel {
		return errors.Wrap(s.stageViaPushBuildSh(), "stage via push-build.sh")
	}

	return errors.Wrap(
		build.NewInstance(&build.Options{
			Bucket:         s.bucket,
			Registry:       s.dockerRegistry,
			GCSRoot:        s.gcsSuffix,
			VersionSuffix:  s.versionSuffix,
			AllowDup:       !s.noAllowDup,
			CI:             s.ci,
			NoUpdateLatest: true,
		}).Push(), "stage via krel push",
	)
}

func (s *stageStrategy) stageViaPushBuildSh() error {
	name := util.K8s("release", "push-build.sh")
	args := []string{
		"--nomock",
		"--verbose",
		"--noupdatelatest", // we may need to expose control of this if build jobs start using kubetest
		fmt.Sprintf("--bucket=%v", s.bucket),
	}
	if s.ci {
		args = append(args, "--ci")
	}
	if len(s.gcsSuffix) > 0 {
		args = append(args, fmt.Sprintf("--gcs-suffix=%v", s.gcsSuffix))
	}
	if len(s.versionSuffix) > 0 {
		args = append(args, fmt.Sprintf("--version-suffix=%s", s.versionSuffix))
	}
	if len(s.dockerRegistry) > 0 {
		args = append(args, fmt.Sprintf("--docker-registry=%s", s.dockerRegistry))
	}

	if !s.noAllowDup {
		args = append(args, "--allow-dup")
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = util.K8s("kubernetes")
	return control.FinishRunning(cmd)
}
