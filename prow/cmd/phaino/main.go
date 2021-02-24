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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/interrupts"
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
	repoPath     map[string]string

	jobs []string
}

func gatherOptions() options {
	var o options
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.BoolVar(&o.keepGoing, "keep-going", false, "Continue running jobs after one fails if set")
	fs.BoolVar(&o.printCmd, "print", false, "Just print the command it would run")
	fs.BoolVar(&o.priv, "privileged", false, "Allow privileged local runs")
	fs.DurationVar(&o.timeout, "timeout", time.Hour, "Maximum duration for each job (0 for unlimited)")
	fs.DurationVar(&o.totalTimeout, "total-timeout", 0, "Maximum duration for all jobs (0 for unlimited)")
	fs.DurationVar(&o.grace, "grace", 10*time.Second, "Terminate timed out jobs after this grace period (1s minimum)")
	fs.StringToStringVar(&o.repoPath, "repo", map[string]string{}, "Local path of repos, can be passed in repeatedly")
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

func readPJ(reader io.ReadCloser) (*prowapi.ProwJob, error) {
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

func readFile(path string) (*prowapi.ProwJob, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	defer f.Close()
	return readPJ(f)

}

func readHTTP(url string) (*prowapi.ProwJob, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}
	return readPJ(resp.Body)
}

func readPJs(jobs []string) (<-chan prowapi.ProwJob, <-chan error) {
	ch := make(chan prowapi.ProwJob)
	errch := make(chan error)
	go func() {
		defer close(ch)
		defer close(errch)
		if len(jobs) == 0 {
			logrus.Info("Converting stdin prowjob...")
			pj, err := readPJ(os.Stdin)
			if err != nil {
				errch <- err
				return
			}
			ch <- *pj
			errch <- nil
			return
		}
		for _, j := range jobs {
			var pj *prowapi.ProwJob
			var err error
			if strings.HasPrefix(j, "https:") || strings.HasPrefix(j, "http:") {
				logrus.WithField("url", j).Info("Downloading...")
				pj, err = readHTTP(j)
			} else {
				logrus.WithField("path", j).Info("Reading...")
				pj, err = readFile(j)
			}
			if err != nil {
				errch <- fmt.Errorf("%q: %v", j, err)
				return
			}
			ch <- *pj
		}
		errch <- nil
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

	pjs, errs := readPJs(opt.jobs)

	defer func() {
		logrus.Info("Press Ctrl + c to exit.")
		interrupts.WaitForGracefulShutdown()
	}()

	if err := processJobs(interrupts.Context(), opt, pjs, errs); err != nil {
		logrus.WithError(err).Fatal("FAILED")
	}
	logrus.Info("SUCCESS")
}

func processJobs(ctx context.Context, opt options, pjs <-chan prowapi.ProwJob, errs <-chan error) error {
	var cancel func()
	if opt.totalTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, opt.totalTimeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	for {
		select {
		case pj := <-pjs:
			start := time.Now()
			log := logrus.WithField("job", jobName(pj))
			// Start job execution.
			err := convertJob(ctx, log, pj, opt.repoPath, opt.priv, opt.printCmd, opt.timeout, opt.grace)
			log = log.WithField("duration", time.Since(start))
			if err != nil {
				log.WithError(err).Error("FAIL")
				if !opt.keepGoing {
					log.Warn("Aborting other jobs...")
					cancel()
				}
				continue
			}
			log.Infof("PASS: %s", pj.Name)
		case err := <-errs:
			return err
		case <-ctx.Done():
			return fmt.Errorf("cancel: %v", ctx.Err())
		}
	}
}
