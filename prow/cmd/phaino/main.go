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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

type options struct {
	keepGoing    bool
	printCmd     bool
	priv         bool
	timeout      time.Duration
	totalTimeout time.Duration
	grace        time.Duration

	jobs []string
}

func gatherOptions() options {
	var o options
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.BoolVar(&o.keepGoing, "keep-going", false, "Continue running jobs after one fails if set")
	fs.BoolVar(&o.printCmd, "print", false, "Just print the command it would run")
	fs.BoolVar(&o.priv, "privileged", false, "Allow privileged local runs")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Minute, "Maximum duration for each job (0 for unlimited)")
	fs.DurationVar(&o.totalTimeout, "total-timeout", 0, "Maximum duration for all jobs (0 for unlimited)")
	fs.DurationVar(&o.grace, "grace", 10*time.Second, "Terminate timed out jobs after this grace period (1s minimum)")
	fs.Parse(os.Args[1:])
	o.jobs = fs.Args()
	return o
}

func validate(pj prowapi.ProwJob) error {
	switch {
	case pj.Kind != "ProwJob":
		return fmt.Errorf("bad kind: %q", pj.Kind)
	case pj.Spec.PodSpec == nil && pj.Spec.Agent != prowapi.KubernetesAgent:
		return fmt.Errorf("unsupported agent: %q. Only %q with a pod_spec is supported at present", pj.Spec.Agent, prowapi.KubernetesAgent)
	case pj.Spec.PodSpec == nil:
		return errors.New("empty pod_spec")
	}
	return nil
}

func readPJ(ctx context.Context, reader io.ReadCloser) (*prowapi.ProwJob, error) {
	read, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-read.Done():
			// finished reading
		case <-ctx.Done():
			logrus.Info("Interrupting read...")
			reader.Close() // interrupt reading
		}
	}()
	var pj prowapi.ProwJob
	buf, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read: %v", err)
	}
	if err := yaml.Unmarshal(buf, &pj); err != nil {
		return nil, fmt.Errorf("unmarshal: %v", err)
	}
	if err := validate(pj); err != nil {
		return nil, fmt.Errorf("validate: %v", err)
	}
	return &pj, nil
}

func readFile(ctx context.Context, path string) (*prowapi.ProwJob, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	defer f.Close()
	return readPJ(ctx, f)

}

func readHTTP(ctx context.Context, url string) (*prowapi.ProwJob, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	return readPJ(ctx, resp.Body)
}

func readPJs(ctx context.Context, jobs []string) (<-chan prowapi.ProwJob, <-chan error) {
	ch := make(chan prowapi.ProwJob)
	errch := make(chan error)
	go func() {
		defer close(ch)
		defer close(errch)
		if len(jobs) == 0 {
			logrus.Info("Converting stdin prowjob...")
			pj, err := readPJ(ctx, os.Stdin)
			if err != nil {
				select {
				case errch <- err:
				case <-ctx.Done():
				}
				return
			}
			select {
			case ch <- *pj:
			case <-ctx.Done():
			}
			return
		}
		for _, j := range jobs {
			var pj *prowapi.ProwJob
			var err error
			if strings.HasPrefix(j, "https:") || strings.HasPrefix(j, "http:") {
				logrus.WithField("url", j).Info("Downloading...")
				pj, err = readHTTP(ctx, j)
			} else {
				logrus.WithField("path", j).Info("Reading...")
				pj, err = readFile(ctx, j)
			}
			if err != nil {
				select {
				case errch <- fmt.Errorf("%q: %v", j, err):
				case <-ctx.Done():
				}
				return
			}
			select {
			case ch <- *pj:
			case <-ctx.Done():
				return
			}
		}
		select {
		case errch <- nil:
		case <-ctx.Done():
		}
	}()
	return ch, errch
}

func jobName(pj prowapi.ProwJob) string {
	if pj.Spec.Job != "" {
		return pj.Spec.Job
	}
	return pj.Name
}

func main() {
	opt := gatherOptions()
	ctx, cancel := context.WithCancel(context.Background())
	pjs, errs := readPJs(ctx, opt.jobs)
	if opt.totalTimeout > 0 {
		var c2 func()
		ctx, c2 = context.WithTimeout(ctx, opt.totalTimeout)
		defer c2()
	}
	for {
		select {
		case pj := <-pjs:
			start := time.Now()
			log := logrus.WithField("job", jobName(pj))
			err := convertJob(ctx, log, pj, opt.priv, opt.printCmd, opt.timeout, opt.grace)
			log = log.WithField("duration", time.Now().Sub(start))
			if err != nil {
				log.WithError(err).Error("FAIL")
				if !opt.keepGoing {
					log.Warn("Aborting other jobs...")
					cancel()
				}
				continue
			}
			log.Info("PASS")
		case err := <-errs:
			if err != nil {
				logrus.WithError(err).Fatal("Error reading jobs")
			}
			return
		case <-ctx.Done():
			logrus.WithError(ctx.Err()).Fatal("Total timeout expired")
		}
	}
}
