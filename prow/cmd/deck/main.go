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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/NYTimes/gziphandler"
	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
)

var (
	configPath   = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	buildCluster = flag.String("build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")
	tideURL      = flag.String("tide-url", "", "Path to tide. If empty, do not serve tide data.")
	hookURL      = flag.String("hook-url", "", "Path to hook plugin help endpoint.")
	// Feature flag for now, can be removed in the future.
	enableTracing = flag.Bool("enable-tracing", false, "Enable log tracing in prow.")
)

// Matches letters, numbers, hyphens, and underscores.
var objReg = regexp.MustCompile(`^[\w-]+$`)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logger := logrus.WithField("component", "deck")

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logger.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logger.WithError(err).Fatal("Error getting client.")
	}
	var pkc *kube.Client
	if *buildCluster == "" {
		pkc = kc.Namespace(configAgent.Config().PodNamespace)
	} else {
		pkc, err = kube.NewClientFromFile(*buildCluster, configAgent.Config().PodNamespace)
		if err != nil {
			logger.WithError(err).Fatal("Error getting kube client to build cluster.")
		}
	}

	ja := &JobAgent{
		kc:  kc,
		pkc: pkc,
		c:   configAgent,
	}
	ja.Start()

	http.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir("/static"))))
	http.Handle("/data.js", gziphandler.GzipHandler(handleData(ja)))
	http.Handle("/prowjobs.js", gziphandler.GzipHandler(handleProwJobs(ja)))
	http.Handle("/log", gziphandler.GzipHandler(handleLog(ja)))
	http.Handle("/rerun", gziphandler.GzipHandler(handleRerun(kc)))
	if *enableTracing {
		http.Handle("/trace", gziphandler.GzipHandler(handleTrace(ja)))
	}

	if *hookURL != "" {
		http.Handle("/plugin-help.js", gziphandler.GzipHandler(handlePluginHelp(newHelpAgent(*hookURL))))
	}

	if *tideURL != "" {
		ta := &tideAgent{
			log:  logger.WithField("agent", "tide"),
			path: *tideURL,
		}
		ta.start()
		http.Handle("/tide.js", gziphandler.GzipHandler(handleTide(ta)))
	}

	logger.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

func loadToken(file string) (string, error) {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(raw)), nil
}

func handleProwJobs(ja *JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		jobs := ja.ProwJobs()
		jd, err := json.Marshal(struct {
			Items []kube.ProwJob `json:"items"`
		}{jobs})
		if err != nil {
			logrus.WithError(err).Error("Error marshaling jobs.")
			jd = []byte("{}")
		}
		// If we have a "var" query, then write out "var value = {...};".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(jd))
		} else {
			fmt.Fprint(w, string(jd))
		}
	}
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
			fmt.Fprint(w, string(jd))
		}
	}
}

func handleTide(ta *tideAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		ta.Lock()
		defer ta.Unlock()
		pd, err := json.Marshal(ta.pools)
		if err != nil {
			logrus.WithError(err).Error("Error marshaling pools.")
			pd = []byte("[]")
		}
		// If we have a "var" query, then write out "var value = [...];".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(pd))
		} else {
			fmt.Fprint(w, string(pd))
		}

	}
}

func handlePluginHelp(ha *helpAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		help, err := ha.getHelp()
		if err != nil {
			logrus.WithError(err).Error("Getting plugin help from hook.")
			help = &pluginhelp.Help{}
		}
		b, err := json.Marshal(*help)
		if err != nil {
			logrus.WithError(err).Error("Marshaling plugin help.")
			b = []byte("[]")
		}
		// If we have a "var" query, then write out "var value = [...];".
		// Otherwise, just write out the JSON.
		if v := r.URL.Query().Get("var"); v != "" {
			fmt.Fprintf(w, "var %s = %s;", v, string(b))
		} else {
			fmt.Fprint(w, string(b))
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
	reader := bufio.NewReader(log)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// TODO(rmmh): The log stops streaming after 30s.
			// This seems to be an apiserver limitation-- investigate?
			// logrus.WithError(err).Error("chunk failed to read!")
			break
		}
		w.Write(line)
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
		if err := validateLogRequest(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
			httpChunking(log, w)
		}
	}
}

func validateLogRequest(r *http.Request) error {
	job := r.URL.Query().Get("job")
	id := r.URL.Query().Get("id")

	if job == "" {
		return errors.New("Missing job query")
	}
	if id == "" {
		return errors.New("Missing ID query")
	}
	if !objReg.MatchString(job) {
		return fmt.Errorf("Invalid job query: %s", job)
	}
	if !objReg.MatchString(id) {
		return fmt.Errorf("Invalid ID query: %s", id)
	}
	return nil
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
		pjutil := pjutil.NewProwJob(pj.Spec, pj.Metadata.Labels)
		b, err := yaml.Marshal(&pjutil)
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
