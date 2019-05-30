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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/test-infra/prow/github"
)

type lineData struct {
	TimeStr   string `json:"time"`
	time      time.Time
	PrNum     int    `json:"pr"`
	Repo      string `json:"repo"`
	Org       string `json:"org"`
	EventGUID string `json:"event-GUID"`
	URL       string `json:"url"`
}

type podLine struct {
	actual      []byte
	unmarshaled lineData
}

// linesByTimestamp sorts pod lines by timestamp.
// Useful when collecting logs across multiple pods.
type linesByTimestamp []podLine

// linesByTimestamp implements the sort.Interface interface.
var _ sort.Interface = linesByTimestamp{}

func (l linesByTimestamp) Len() int      { return len(l) }
func (l linesByTimestamp) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l linesByTimestamp) Less(i, j int) bool {
	return l[i].unmarshaled.time.Before(l[j].unmarshaled.time)
}

// linesByTimestamp implements the fmt.Stringer interface.
var _ fmt.Stringer = linesByTimestamp{}

// Return valid json.
func (l linesByTimestamp) String() string {
	sort.Sort(l)

	var log string
	for i, line := range l {
		switch i {
		case len(l) - 1:
			log += string(line.actual)
		default:
			// buf.ReadBytes('\n') does not remove the newline
			log += fmt.Sprintf("%s,\n", strings.TrimSuffix(string(line.actual), "\n"))
		}
	}

	return fmt.Sprintf("[%s]", log)
}

// ic is the prefix of a URL fragment for a GitHub comment.
// The URL fragment has the following format:
//
// issuecomment-#id
//
// We use #id in order to figure out the event-GUID for a
// comment and trace comments across prow.
const ic = "issuecomment"

func handleTrace(selector string, client corev1.PodInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logrus.Info("Started handling request")
		start := time.Now()
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		if err := validateTraceRequest(r); err != nil {
			logrus.Infof("Invalid request: %v", err)
			http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
			return
		}

		targets, err := getPods(selector, client)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if len(targets) == 0 {
			logrus.Info("No targets found.")
			fmt.Fprint(w, "[]")
			return
		}

		// Return the logs from the found targets.
		log := getPodLogs(client, targets, r)
		// Filter further if issuecomment has been provided.
		fmt.Fprint(w, postFilter(log, r.URL.Query().Get(ic)))
		logrus.Info("Finished handling request")
		logrus.Infof("Sync time: %v", time.Since(start))
	}
}

func validateTraceRequest(r *http.Request) error {
	icID := r.URL.Query().Get(ic)
	pr := r.URL.Query().Get(github.PrLogField)
	repo := r.URL.Query().Get(github.RepoLogField)
	org := r.URL.Query().Get(github.OrgLogField)
	eventGUID := r.URL.Query().Get(github.EventGUID)

	if (pr == "" || repo == "" || org == "") && eventGUID == "" && icID == "" {
		return fmt.Errorf("need either %q, %q, and %q, or %q, or %q to be specified",
			github.PrLogField, github.RepoLogField, github.OrgLogField, github.EventGUID, ic)
	}
	if icID != "" && eventGUID != "" {
		return fmt.Errorf("cannot specify both %s (%s) and %s (%s)", ic, icID, github.EventGUID, eventGUID)
	}
	var prNum int
	if pr != "" {
		var err error
		prNum, err = strconv.Atoi(pr)
		if err != nil {
			return fmt.Errorf("invalid pr query %q: %v", pr, err)
		}
		if prNum < 1 {
			return fmt.Errorf("invalid pr query %q: needs to be a positive number", pr)
		}
	}
	return nil
}

func getPods(selector string, client corev1.PodInterface) ([]coreapi.Pod, error) {
	pods, err := client.List(metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, fmt.Errorf("Cannot list pods with selector %q: %v", selector, err)
	}
	var targets []coreapi.Pod
	for _, pod := range pods.Items {
		if pod.Status.Phase != coreapi.PodRunning {
			logrus.Warnf("Ignoring pod %q: not in %s phase (phase: %s, reason: %s)",
				pod.ObjectMeta.Name, coreapi.PodRunning, pod.Status.Phase, pod.Status.Reason)
			continue
		}
		targets = append(targets, pod)
	}
	return targets, nil
}

func getPodLogs(client corev1.PodInterface, targets []coreapi.Pod, r *http.Request) linesByTimestamp {
	var lock sync.Mutex
	log := make(linesByTimestamp, 0)
	wg := sync.WaitGroup{}
	wg.Add(len(targets))
	for _, pod := range targets {
		go func(podName string) {
			defer wg.Done()
			podLog, err := getPodLog(podName, client, r)
			if err != nil {
				logrus.Warnf("cannot get logs from %q: %v", podName, err)
				return
			}
			lock.Lock()
			log = append(log, podLog...)
			lock.Unlock()
		}(pod.ObjectMeta.Name)
	}
	wg.Wait()
	return log
}

func getPodLog(podName string, client corev1.PodInterface, r *http.Request) (linesByTimestamp, error) {
	pr := r.URL.Query().Get(github.PrLogField)
	// Error already checked in validateTraceRequest
	prNum, _ := strconv.Atoi(pr)
	repo := r.URL.Query().Get(github.RepoLogField)
	org := r.URL.Query().Get(github.OrgLogField)
	eventGUID := r.URL.Query().Get(github.EventGUID)

	podLog, err := client.GetLogs(podName, &coreapi.PodLogOptions{}).DoRaw()
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(podLog)

	log := make(linesByTimestamp, 0)
	for {
		line, err := buf.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			logrus.Debugf("error while reading log line from %q: %v", podName, err)
			continue
		}

		var jsonLine lineData
		if err := json.Unmarshal(line, &jsonLine); err != nil {
			logrus.Debugf("cannot unmarshal log line from %q (%s): %v", podName, string(line), err)
			continue
		}
		if eventGUID != "" && jsonLine.EventGUID != eventGUID {
			continue
		}
		if pr != "" && jsonLine.PrNum != prNum {
			continue
		}
		if repo != "" && jsonLine.Repo != repo {
			continue
		}
		if org != "" && jsonLine.Org != org {
			continue
		}
		jsonLine.time, err = time.Parse(time.RFC3339, jsonLine.TimeStr)
		if err != nil {
			logrus.Debugf("could not parse time format: %v", err)
			// Continue including this in the output at the expense
			// of not having it sorted.
		}
		log = append(log, podLine{actual: line, unmarshaled: jsonLine})
	}
	return log, nil
}

func postFilter(log linesByTimestamp, icID string) linesByTimestamp {
	if icID == "" {
		return log
	}
	var icEventGUID string
	for _, l := range log {
		if strings.HasSuffix(l.unmarshaled.URL, fmt.Sprintf("%s-%s", ic, icID)) {
			icEventGUID = l.unmarshaled.EventGUID
			break
		}
	}
	// No event-GUID was found for the provided issuecomment.
	// No logs should be returned.
	if icEventGUID == "" {
		return linesByTimestamp{}
	}

	filtered := make(linesByTimestamp, 0)
	for _, l := range log {
		if l.unmarshaled.EventGUID != icEventGUID {
			continue
		}
		filtered = append(filtered, l)
	}
	return filtered
}
