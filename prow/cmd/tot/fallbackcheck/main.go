/*
Copyright 2018 The Kubernetes Authors.

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

// fallbackcheck reports whether jobs in the provided prow
// deployment have fallback build numbers in GCS.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

type options struct {
	prowURL string
	bucket  string
}

func gatherOptions() options {
	o := options{}

	// TODO: Support reading from a local file.
	flag.StringVar(&o.prowURL, "prow-url", "", "Prow frontend URL.")
	flag.StringVar(&o.bucket, "bucket", "", "Top-level GCS bucket where job artifacts are pushed.")

	flag.Parse()
	return o
}

func (o *options) Validate() error {
	if o.prowURL == "" {
		return errors.New("you need to provide a URL to a live prow deployment")
	}
	if o.bucket == "" {
		return errors.New("you need to provide the GCS bucket where all job artifacts are pushed")
	}
	return nil
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	// TODO: Retries
	resp, err := http.Get(o.prowURL + "/config")
	if err != nil {
		logrus.Fatalf("cannot get prow config: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		logrus.Fatalf("status code not 2XX: %v", resp.Status)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.Fatalf("cannot read request body: %v", err)
	}

	cfg := &config.Config{}
	if err := yaml.UnmarshalStrict(data, &cfg); err != nil {
		logrus.Fatalf("cannot unmarshal data from prow: %v", err)
	}

	var notFound bool
	for _, pre := range cfg.AllStaticPresubmits(nil) {
		spec := pjutil.PresubmitToJobSpec(pre)
		nf, err := getJobFallbackNumber(o.bucket, spec)
		if err != nil {
			logrus.Fatalf("cannot get fallback number: %v", err)
		}
		notFound = notFound || nf
	}
	for _, post := range cfg.AllStaticPostsubmits(nil) {
		spec := pjutil.PostsubmitToJobSpec(post)
		nf, err := getJobFallbackNumber(o.bucket, spec)
		if err != nil {
			logrus.Fatalf("cannot get fallback number: %v", err)
		}
		notFound = notFound || nf
	}

	for _, per := range cfg.AllPeriodics() {
		spec := pjutil.PeriodicToJobSpec(per)
		nf, err := getJobFallbackNumber(o.bucket, spec)
		if err != nil {
			logrus.Fatalf("cannot get fallback number: %v", err)
		}
		notFound = notFound || nf
	}

	if notFound {
		os.Exit(1)
	}
}

func getJobFallbackNumber(bucket string, spec *downwardapi.JobSpec) (bool, error) {
	url := fmt.Sprintf("%s/%s", strings.TrimSuffix(bucket, "/"), gcs.LatestBuildForSpec(spec, nil)[0])

	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		fmt.Printf("OK: %s\n", spec.Job)
		return false, nil
	case http.StatusNotFound:
		rootURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(bucket, "/"), gcs.RootForSpec(spec))
		resp, err := http.Get(rootURL)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			fmt.Printf("NOT FOUND: %s\n", spec.Job)
			return true, nil
		case http.StatusNotFound:
			fmt.Printf("IGNORE: %s\n", spec.Job)
			return false, nil
		default:
			return false, fmt.Errorf("unexpected status when checking the existence of %s: %v", spec.Job, resp.Status)
		}

	default:
		return false, fmt.Errorf("unexpected status for %s: %v", spec.Job, resp.Status)
	}
}
