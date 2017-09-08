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

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"

	"github.com/NYTimes/gziphandler"
	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/npj"
)

var (
	configPath   = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	buildCluster = flag.String("build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")

	jenkinsURL             = flag.String("jenkins-url", "", "Jenkins URL")
	jenkinsUserName        = flag.String("jenkins-user", "jenkins-trigger", "Jenkins username")
	jenkinsTokenFile       = flag.String("jenkins-token-file", "", "Path to the file containing the Jenkins API token.")
	jenkinsBearerTokenFile = flag.String("jenkins-bearer-token-file", "", "Path to the file containing the Jenkins API bearer token.")
)

// Matches letters, numbers, hyphens, and underscores.
var objReg = regexp.MustCompile(`^[\w-]+$`)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting client.")
	}
	var pkc *kube.Client
	if *buildCluster == "" {
		pkc = kc.Namespace(configAgent.Config().PodNamespace)
	} else {
		pkc, err = kube.NewClientFromFile(*buildCluster, configAgent.Config().PodNamespace)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting kube client to build cluster.")
		}
	}

	ac := &jenkins.AuthConfig{}
	var jc *jenkins.Client
	if *jenkinsURL != "" {
		if *jenkinsTokenFile != "" {
			if token, err := loadToken(*jenkinsTokenFile); err != nil {
				logrus.WithError(err).Fatalf("Could not read token file.")
			} else {
				ac.Basic = &jenkins.BasicAuthConfig{
					User:  *jenkinsUserName,
					Token: token,
				}
			}
		} else if *jenkinsBearerTokenFile != "" {
			if token, err := loadToken(*jenkinsBearerTokenFile); err != nil {
				logrus.WithError(err).Fatalf("Could not read token file.")
			} else {
				ac.BearerToken = &jenkins.BearerTokenAuthConfig{
					Token: token,
				}
			}
		} else {
			logrus.Fatal("An auth token for basic or bearer token auth must be supplied.")
		}
		jc = jenkins.NewClient(*jenkinsURL, ac)
	}

	ja := &JobAgent{
		kc:  kc,
		pkc: pkc,
		jc:  jc,
	}
	ja.Start()

	http.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir("/static"))))
	http.Handle("/data.js", gziphandler.GzipHandler(handleData(ja)))
	http.Handle("/log", gziphandler.GzipHandler(handleLog(ja)))
	http.Handle("/rerun", gziphandler.GzipHandler(handleRerun(kc)))

	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

func loadToken(file string) (string, error) {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(raw)), nil
}

func handleData(ja *JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		jobs := ja.Jobs()
		jd, err := json.Marshal(jobs)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling jobs.")
			jd = []byte("[]")
		}
		// If we have a "var" query, then write out "var value = {...};".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(jd))
		} else {
			fmt.Fprintf(w, string(jd))
		}
	}
}

type logClient interface {
	GetJobLog(job, id string) ([]byte, error)
	// Add ability to stream logs with options enabled. This call is used to follow logs
	// using kubernetes client API. All other options on the Kubernetes log api can
	// also be enabled.
	GetJobLogStream(job, id string, options map[string]string) (io.ReadCloser, error)
}

func httpChunking(log io.ReadCloser, w http.ResponseWriter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		logrus.Warning("Error getting flusher.")
	}
	cw := httputil.NewChunkedWriter(w)
	reader := bufio.NewReader(log)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
		}
		cw.Write(line)
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func getOptions(values url.Values) map[string]string {
	options := make(map[string]string)
	for k, v := range values {
		if k != "pod" && k != "job" && k != "id" {
			options[k] = v[0]
		}
	}
	return options
}

// TODO(spxtr): Cache, rate limit.
func handleLog(lc logClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		job := r.URL.Query().Get("job")
		id := r.URL.Query().Get("id")
		stream := r.URL.Query().Get("follow")
		var logStreamRequested bool
		if ok, _ := strconv.ParseBool(stream); ok {
			// get http chunked responses to the client
			w.Header().Set("Connection", "Keep-Alive")
			w.Header().Set("Transfer-Encoding", "chunked")
			logStreamRequested = true
		}
		if job != "" && id != "" {
			if !objReg.MatchString(job) {
				http.Error(w, "Invalid job query", http.StatusBadRequest)
				return
			}
			if !objReg.MatchString(id) {
				http.Error(w, "Invalid ID query", http.StatusBadRequest)
				return
			}
			if !logStreamRequested {
				log, err := lc.GetJobLog(job, id)
				if err != nil {
					http.Error(w, fmt.Sprintf("Log not found: %v", err), http.StatusNotFound)
					logrus.WithError(err).Warning("Error returned.")
					return
				}
				if _, err = w.Write(log); err != nil {
					logrus.WithError(err).Warning("Error writing log.")
				}
			} else {
				//run http chunking
				options := getOptions(r.URL.Query())
				log, err := lc.GetJobLogStream(job, id, options)
				if err != nil {
					http.Error(w, fmt.Sprintf("Log stream caused: %v", err), http.StatusNotFound)
					logrus.WithError(err).Warning("Error returned.")
					return
				}
				go httpChunking(log, w)
			}
		} else {
			http.Error(w, "Missing job and ID query", http.StatusBadRequest)
			return
		}
	}
}

type pjClient interface {
	GetProwJob(string) (kube.ProwJob, error)
}

func handleRerun(kc pjClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("prowjob")
		if !objReg.MatchString(name) {
			http.Error(w, "Invalid ProwJob query", http.StatusBadRequest)
			return
		}
		pj, err := kc.GetProwJob(name)
		if err != nil {
			http.Error(w, fmt.Sprintf("ProwJob not found: %v", err), http.StatusNotFound)
			logrus.WithError(err).Warning("Error returned.")
			return
		}
		npj := npj.NewProwJob(pj.Spec)
		b, err := yaml.Marshal(&npj)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error marshaling: %v", err), http.StatusInternalServerError)
			logrus.WithError(err).Error("Error marshaling jobs.")
			return
		}
		if _, err := w.Write(b); err != nil {
			logrus.WithError(err).Error("Error writing log.")
		}
	}
}
