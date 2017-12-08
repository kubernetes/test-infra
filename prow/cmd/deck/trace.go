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
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
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
func (pl linesByTimestamp) String() string {
	sort.Sort(pl)

	var log string
	for i, line := range pl {
		switch i {
		case len(pl) - 1:
			log += string(line.actual)
		default:
			// buf.ReadBytes('\n') does not remove the newline
			log += fmt.Sprintf("%s,\n", strings.TrimSuffix(string(line.actual), "\n"))
		}
	}

	return fmt.Sprintf("[%s]", log)
}

// ic is the prefix of a URL fragment for a Github comment.
// The URL fragment has the following format:
//
// issuecomment-#id
//
// We use #id in order to figure out the event-GUID for a
// comment and trace commments across prow.
const ic = "issuecomment"

func handleTrace(ja *JobAgent) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		prNum, err := validateTraceRequest(r)
		if err != nil {
			logrus.Debugf("Invalid request: %v", err)
			http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
			return
		}

		var targets []kube.Pod
		for _, selector := range ja.c.Config().Deck.TraceTargets {
			pods, err := ja.pkc.ListPods(selector)
			if err != nil {
				logrus.Debugf("Cannot list pods with selector %q: %v", selector, err)
				http.Error(w, fmt.Sprintf("Cannot list pods with selector %q: %v", selector, err), http.StatusBadGateway)
				return
			}
			for _, pod := range pods {
				if pod.Status.Phase != kube.PodRunning {
					logrus.Debugf("Ignoring pod %q: not in %s phase (phase: %s, reason: %s)",
						pod.Metadata.Name, kube.PodRunning, pod.Status.Phase, pod.Status.Reason)
					continue
				}
				targets = append(targets, pod)
			}
		}

		if len(targets) == 0 {
			logrus.Debug("No targets found.")
			fmt.Fprint(w, "[]")
			return
		}

		pr := r.URL.Query().Get(github.PrLogField)
		repo := r.URL.Query().Get(github.RepoLogField)
		org := r.URL.Query().Get(github.OrgLogField)
		eventGUID := r.URL.Query().Get(github.EventGUID)

		icID := r.URL.Query().Get(ic)
		var icChecked bool
		var icEventGUID string

		log := make(linesByTimestamp, 0)
		var buf *bytes.Buffer
		for _, pod := range targets {
			// TODO: Cache this and use "since" as a pod/log url parameter to fetch
			// newer logs.
			podLog, err := ja.pkc.GetLog(pod.Metadata.Name)
			if err != nil {
				logrus.Debugf("cannot get logs from %q: %v", pod.Metadata.Name, err)
				continue
			}
			buf = bytes.NewBuffer(podLog)

			for {
				line, err := buf.ReadBytes('\n')
				if err == io.EOF {
					break
				}
				if err != nil {
					logrus.Debugf("error while reading log line from %q: %v", pod.Metadata.Name, err)
					continue
				}

				var jsonLine lineData
				if err := json.Unmarshal(line, &jsonLine); err != nil {
					logrus.Debugf("cannot unmarshal log line from %q (%s): %v", pod.Metadata.Name, string(line), err)
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
				// If issuecomment is specified, figure out its event-GUID and
				// and select logs based on the event-GUID.
				if icID != "" && !icChecked && strings.HasSuffix(jsonLine.URL, fmt.Sprintf("%s-%s", ic, icID)) {
					icChecked = true
					icEventGUID = jsonLine.EventGUID
				}
				jsonLine.time, err = time.Parse(time.RFC3339, jsonLine.TimeStr)
				if err != nil {
					logrus.Debugf("could not parse time format: %v", err)
					// Continue including this in the output at the expense
					// of not having it sorted.
				}
				log = append(log, podLine{actual: line, unmarshaled: jsonLine})
			}
		}

		// No event-GUID was found for the provided issuecomment.
		// No logs should be returned.
		if icID != "" && icEventGUID == "" {
			fmt.Fprint(w, "[]")
			return
		}

		if icEventGUID != "" {
			tmpLog := make(linesByTimestamp, 0)
			for _, l := range log {
				if l.unmarshaled.EventGUID != icEventGUID {
					continue
				}
				tmpLog = append(tmpLog, l)
			}
			log = tmpLog
		}

		fmt.Fprint(w, log)
	}
}

func validateTraceRequest(r *http.Request) (int, error) {
	icID := r.URL.Query().Get(ic)
	pr := r.URL.Query().Get(github.PrLogField)
	repo := r.URL.Query().Get(github.RepoLogField)
	org := r.URL.Query().Get(github.OrgLogField)
	eventGUID := r.URL.Query().Get(github.EventGUID)

	if (pr == "" || repo == "" || org == "") && eventGUID == "" && icID == "" {
		return 0, fmt.Errorf("need either %q, %q, and %q, or %q, or %q to be specified",
			github.PrLogField, github.RepoLogField, github.OrgLogField, github.EventGUID, ic)
	}
	if icID != "" && eventGUID != "" {
		return 0, fmt.Errorf("cannot specify both %s (%s) and %s (%s)", ic, icID, github.EventGUID, eventGUID)
	}
	var prNum int
	if pr != "" {
		var err error
		prNum, err = strconv.Atoi(pr)
		if err != nil {
			return 0, fmt.Errorf("invalid pr query %q: %v", pr, err)
		}
		if prNum < 1 {
			return 0, fmt.Errorf("invalid pr query %q: needs to be a positive number", pr)
		}
	}
	return prNum, nil
}
