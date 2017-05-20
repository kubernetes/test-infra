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

package config

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
)

func TestConfigLoads(t *testing.T) {
	_, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
}

func Replace(j *Presubmit, ks *Presubmit) error {
	name := strings.Replace(j.Name, "pull-kubernetes", "pull-security-kubernetes", -1)
	if name != ks.Name {
		return fmt.Errorf("%s should match %s", name, ks.Name)
	}
	j.Name = name
	j.RerunCommand = strings.Replace(j.RerunCommand, "pull-kubernetes", "pull-security-kubernetes", -1)
	j.Trigger = strings.Replace(j.Trigger, "pull-kubernetes", "pull-security-kubernetes", -1)
	j.Context = strings.Replace(j.Context, "pull-kubernetes", "pull-security-kubernetes", -1)
	j.re = ks.re
	if len(j.RunAfterSuccess) != len(ks.RunAfterSuccess) {
		return fmt.Errorf("RunAfterSuccess should match. - %s", name)
	}

	for i := range j.RunAfterSuccess {
		if err := Replace(&j.RunAfterSuccess[i], &ks.RunAfterSuccess[i]); err != nil {
			return err
		}
	}

	return nil
}

func CheckContext(t *testing.T, repo string, p Presubmit) {
	if p.Name != p.Context {
		t.Errorf("Context does not match job name: %s in %s", p.Name, repo)
	}
	for _, c := range p.RunAfterSuccess {
		CheckContext(t, repo, c)
	}
}

func TestContextMatches(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}

	for repo, presubmits := range c.Presubmits {
		for _, p := range presubmits {
			CheckContext(t, repo, p)
		}
	}
}

type SubmitQueueConfig struct {
	Data map[string]string `json:"data"`
}

func FindRequired(t *testing.T, presubmits []Presubmit) []string {
	var required []string
	for _, p := range presubmits {
		if !p.AlwaysRun {
			continue
		}
		for _, r := range FindRequired(t, p.RunAfterSuccess) {
			required = append(required, r)
		}
		if p.SkipReport {
			continue
		}
		required = append(required, p.Context)
	}
	return required
}

func TestRequiredRetestContextsMatch(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	b, err := ioutil.ReadFile("../../mungegithub/submit-queue/deployment/kubernetes/configmap.yaml")
	if err != nil {
		t.Fatalf("Could not load submit queue configmap: %v", err)
	}
	sqc := &SubmitQueueConfig{}
	if err = yaml.Unmarshal(b, sqc); err != nil {
		t.Fatalf("Could not parse submit queue configmap: %v", err)
	}
	re := regexp.MustCompile(`"([^"]+)"`)
	var required []string
	for _, g := range re.FindAllStringSubmatch(sqc.Data["test-options.required-retest-contexts"], -1) {
		required = append(required, g[1])
	}

	running := FindRequired(t, c.Presubmits["kubernetes/kubernetes"])

	for _, r := range required {
		found := false
		for _, s := range running {
			if s == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Required context: %s does not always run: %s", r, running)
		}
	}

}

func TestConfigSecurityJobsMatch(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	kp := c.Presubmits["kubernetes/kubernetes"]
	sp := c.Presubmits["kubernetes-security/kubernetes"]
	if len(kp) != len(sp) {
		t.Fatalf("length of kubernetes/kubernetes presumits %d does not equal length of kubernetes-security/kubernetes presubmits %d", len(kp), len(sp))
	}
	for i, j := range kp {
		if err := Replace(&j, &sp[i]); err != nil {
			t.Fatalf("[Replace] : %v", err)
		}

		if !reflect.DeepEqual(j, sp[i]) {
			t.Fatalf("kubernetes/kubernetes prow config jobs do not match kubernetes-security/kubernetes jobs:\n%#v\nshould match: %#v", j, sp[i])
		}
	}
}
