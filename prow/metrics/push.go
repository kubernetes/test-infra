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

// This file is a copy from upstream where it was removed in
// https://github.com/prometheus/client_golang/pull/600
package metrics

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const contentTypeHeader = "Content-Type"

func fromGatherer(job string, grouping map[string]string, url string, g prometheus.Gatherer) error {
	return push(job, grouping, url, g, "PUT")
}

func push(job string, grouping map[string]string, pushURL string, g prometheus.Gatherer, method string) error {
	if !strings.Contains(pushURL, "://") {
		pushURL = "http://" + pushURL
	}
	if strings.HasSuffix(pushURL, "/") {
		pushURL = pushURL[:len(pushURL)-1]
	}

	if strings.Contains(job, "/") {
		return fmt.Errorf("job contains '/': %s", job)
	}
	urlComponents := []string{url.QueryEscape(job)}
	for ln, lv := range grouping {
		if !model.LabelName(ln).IsValid() {
			return fmt.Errorf("grouping label has invalid name: %s", ln)
		}
		if strings.Contains(lv, "/") {
			return fmt.Errorf("value of grouping label %s contains '/': %s", ln, lv)
		}
		urlComponents = append(urlComponents, ln, lv)
	}
	pushURL = fmt.Sprintf("%s/metrics/job/%s", pushURL, strings.Join(urlComponents, "/"))

	mfs, err := g.Gather()
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	enc := expfmt.NewEncoder(buf, expfmt.FmtProtoDelim)
	// Check for pre-existing grouping labels:
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "job" {
					return fmt.Errorf("pushed metric %s (%s) already contains a job label", mf.GetName(), m)
				}
				if _, ok := grouping[l.GetName()]; ok {
					return fmt.Errorf(
						"pushed metric %s (%s) already contains grouping label %s",
						mf.GetName(), m, l.GetName(),
					)
				}
			}
		}
		enc.Encode(mf)
	}
	req, err := http.NewRequest(method, pushURL, buf)
	if err != nil {
		return err
	}
	req.Header.Set(contentTypeHeader, string(expfmt.FmtProtoDelim))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 202 {
		body, _ := ioutil.ReadAll(resp.Body) // Ignore any further error as this is for an error message only.
		return fmt.Errorf("unexpected status code %d while pushing to %s: %s", resp.StatusCode, pushURL, body)
	}
	return nil
}

func hostnameGroupingKey() map[string]string {
	hostname, err := os.Hostname()
	if err != nil {
		return map[string]string{"instance": "unknown"}
	}
	return map[string]string{"instance": hostname}
}
