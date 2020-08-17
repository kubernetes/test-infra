/*
Copyright 2016 The Kubernetes Authors.

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

// Tot vends (rations) incrementing numbers for use in builds.
// https://en.wikipedia.org/wiki/Rum_ration
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	"k8s.io/test-infra/prow/pod-utils/gcs"
)

type options struct {
	port        int
	storagePath string

	useFallback bool
	fallbackURI string

	configPath             string
	jobConfigPath          string
	fallbackBucket         string
	instrumentationOptions prowflagutil.InstrumentationOptions
}

func gatherOptions() options {
	o := options{}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")
	flag.StringVar(&o.storagePath, "storage", "tot.json", "Where to store the results.")

	flag.BoolVar(&o.useFallback, "fallback", false, "Fallback to GCS bucket for missing builds.")
	flag.StringVar(&o.fallbackURI, "fallback-url-template",
		"https://storage.googleapis.com/kubernetes-jenkins/logs/%s/latest-build.txt",
		"URL template to fallback to for jobs that lack a last vended build number.",
	)

	flag.StringVar(&o.configPath, "config-path", "", "Path to prow config.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.fallbackBucket, "fallback-bucket", "",
		"Fallback to top-level bucket for jobs that lack a last vended build number. The bucket layout is expected to follow https://github.com/kubernetes/test-infra/tree/master/gubernator#gcs-bucket-layout",
	)
	o.instrumentationOptions.AddFlags(flag.CommandLine)
	flag.Parse()
	return o
}

func (o *options) Validate() error {
	if o.configPath != "" && o.fallbackBucket == "" {
		return errors.New("you need to provide a bucket to fallback to when the prow config is specified")
	}
	if o.configPath == "" && o.fallbackBucket != "" {
		return errors.New("you need to provide the prow config when a fallback bucket is specified")
	}
	return nil
}

type store struct {
	Number       map[string]int // job name -> last vended build number
	mutex        sync.Mutex
	storagePath  string
	fallbackFunc func(string) int
}

func newStore(storagePath string) (*store, error) {
	s := &store{
		Number:      make(map[string]int),
		storagePath: storagePath,
	}
	buf, err := ioutil.ReadFile(storagePath)
	if err == nil {
		err = json.Unmarshal(buf, s)
		if err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *store) save() error {
	buf, err := json.Marshal(s)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(s.storagePath+".tmp", buf, 0644)
	if err != nil {
		return err
	}
	return os.Rename(s.storagePath+".tmp", s.storagePath)
}

func (s *store) vend(jobName string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	n, ok := s.Number[jobName]
	if !ok && s.fallbackFunc != nil {
		n = s.fallbackFunc(jobName)
	}
	n++

	s.Number[jobName] = n

	err := s.save()
	if err != nil {
		logrus.Error(err)
	}

	return n
}

func (s *store) peek(jobName string) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.Number[jobName]
}

func (s *store) set(jobName string, n int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.Number[jobName] = n

	err := s.save()
	if err != nil {
		logrus.Error(err)
	}
}

func (s *store) handle(w http.ResponseWriter, r *http.Request) {
	jobName := r.URL.Path[len("/vend/"):]
	switch r.Method {
	case "GET":
		n := s.vend(jobName)
		logrus.Infof("Vending %s number %d to %s.", jobName, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	case "HEAD":
		n := s.peek(jobName)
		logrus.Infof("Peeking %s number %d to %s.", jobName, n, r.RemoteAddr)
		fmt.Fprintf(w, "%d", n)
	case "POST":
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			logrus.WithError(err).Error("Unable to read body.")
			return
		}
		n, err := strconv.Atoi(string(body))
		if err != nil {
			logrus.WithError(err).Error("Unable to parse number.")
			return
		}
		logrus.Infof("Setting %s to %d from %s.", jobName, n, r.RemoteAddr)
		s.set(jobName, n)
	}
}

type fallbackHandler struct {
	template string
	// in case a config agent is provided, tot will
	// determine the GCS path that it needs to use
	// based on the configured jobs in prow and
	// bucket.
	configAgent *config.Agent
	bucket      string
}

func (f fallbackHandler) get(jobName string) int {
	url := f.getURL(jobName)

	var body []byte

	for i := 0; i < 10; i++ {
		resp, err := http.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, err = ioutil.ReadAll(resp.Body)
				if err == nil {
					break
				} else {
					logrus.WithError(err).Error("Failed to read response body.")
				}
			} else if resp.StatusCode == http.StatusNotFound {
				break
			}
		} else {
			logrus.WithError(err).Errorf("Failed to GET %s.", url)
		}
		time.Sleep(2 * time.Second)
	}

	n, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return 0
	}

	return n
}

func (f fallbackHandler) getURL(jobName string) string {
	if f.configAgent == nil {
		return fmt.Sprintf(f.template, jobName)
	}

	var spec *downwardapi.JobSpec
	cfg := f.configAgent.Config()

	for _, pre := range cfg.AllStaticPresubmits(nil) {
		if jobName == pre.Name {
			spec = pjutil.PresubmitToJobSpec(pre)
			break
		}
	}
	if spec == nil {
		for _, post := range cfg.AllStaticPostsubmits(nil) {
			if jobName == post.Name {
				spec = pjutil.PostsubmitToJobSpec(post)
				break
			}
		}
	}
	if spec == nil {
		for _, per := range cfg.AllPeriodics() {
			if jobName == per.Name {
				spec = pjutil.PeriodicToJobSpec(per)
				break
			}
		}
	}
	// If spec is still nil, we know nothing about the requested job.
	if spec == nil {
		logrus.Errorf("requested job is unknown to prow: %s", jobName)
		return ""
	}
	paths := gcs.LatestBuildForSpec(spec, nil)
	if len(paths) != 1 {
		logrus.Errorf("expected a single GCS path, got %v", paths)
		return ""
	}
	return fmt.Sprintf("%s/%s", strings.TrimSuffix(f.bucket, "/"), paths[0])
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)
	health := pjutil.NewHealth()

	s, err := newStore(o.storagePath)
	if err != nil {
		logrus.WithError(err).Fatal("newStore failed")
	}

	if o.useFallback {
		var configAgent *config.Agent
		if o.configPath != "" {
			configAgent = &config.Agent{}
			if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
				logrus.WithError(err).Fatal("Error starting config agent.")
			}
		}

		s.fallbackFunc = fallbackHandler{
			template:    o.fallbackURI,
			configAgent: configAgent,
			bucket:      o.fallbackBucket,
		}.get
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/vend/", s.handle)
	server := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: mux}
	health.ServeReady()
	interrupts.ListenAndServe(server, 5*time.Second)
}
