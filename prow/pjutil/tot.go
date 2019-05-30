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

package pjutil

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	"github.com/bwmarrin/snowflake"
)

var (
	node  *snowflake.Node
	sleep = time.Sleep
)

func init() {
	var err error
	node, err = snowflake.NewNode(1)
	if err != nil {
		log.Fatalf("failed to register snowflake node: %v", err)
	}
}

// PresubmitToJobSpec generates a downwardapi.JobSpec out of a Presubmit.
// Useful for figuring out GCS paths when parsing jobs out
// of a prow config.
func PresubmitToJobSpec(pre config.Presubmit) *downwardapi.JobSpec {
	return &downwardapi.JobSpec{
		Type: prowapi.PresubmitJob,
		Job:  pre.Name,
	}
}

// PostsubmitToJobSpec generates a downwardapi.JobSpec out of a Postsubmit.
// Useful for figuring out GCS paths when parsing jobs out
// of a prow config.
func PostsubmitToJobSpec(post config.Postsubmit) *downwardapi.JobSpec {
	return &downwardapi.JobSpec{
		Type: prowapi.PostsubmitJob,
		Job:  post.Name,
	}
}

// PeriodicToJobSpec generates a downwardapi.JobSpec out of a Periodic.
// Useful for figuring out GCS paths when parsing jobs out
// of a prow config.
func PeriodicToJobSpec(periodic config.Periodic) *downwardapi.JobSpec {
	return &downwardapi.JobSpec{
		Type: prowapi.PeriodicJob,
		Job:  periodic.Name,
	}
}

// GetBuildID calls out to `tot` in order
// to vend build identifier for the job
func GetBuildID(name, totURL string) (string, error) {
	if totURL == "" {
		return node.Generate().String(), nil
	}
	var err error
	url, err := url.Parse(totURL)
	if err != nil {
		return "", fmt.Errorf("invalid tot url: %v", err)
	}
	url.Path = path.Join(url.Path, "vend", name)
	sleepDuration := 100 * time.Millisecond
	for retries := 0; retries < 10; retries++ {
		if retries > 0 {
			sleep(sleepDuration)
			sleepDuration = sleepDuration * 2
		}
		var resp *http.Response
		resp, err = http.Get(url.String())
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			err = fmt.Errorf("got unexpected response from tot: %v", resp.Status)
			continue
		}
		var buf []byte
		buf, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			return string(buf), nil
		}
		return "", err
	}
	return "", err
}
