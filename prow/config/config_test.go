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
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
	"k8s.io/test-infra/prow/kube"
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
		return fmt.Errorf("length of RunAfterSuccess should match. - %s", name)
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

func CheckRetest(t *testing.T, repo string, presubmits []Presubmit) {
	for _, p := range presubmits {
		expected := fmt.Sprintf("@k8s-bot %s test this", p.Name)
		if p.RerunCommand != expected {
			t.Errorf("%s in %s rerun_command: %s != expected: %s", repo, p.Name, p.RerunCommand, expected)
		}
		CheckRetest(t, repo, p.RunAfterSuccess)
	}
}

func TestRetestMatchJobsName(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}
	for repo, presubmits := range c.Presubmits {
		CheckRetest(t, repo, presubmits)
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

func CheckBazelPortContainer(c kube.Container, cache bool) error {
	if !cache {
		if len(c.Ports) != 0 {
			return errors.New("job does not use --cache-ssd and so should not set ports in spec")
		}
		return nil
	}

	if len(c.Ports) != 1 {
		return errors.New("job uses --cache-ssd and so needs to set ports in spec!")
	} else if c.Ports[0].ContainerPort != 9999 {
		return errors.New("job uses --cache-ssd and so needs to have ContainerPort 9999!")
	} else if c.Ports[0].HostPort != 9999 {
		return errors.New("job uses --cache-ssd and so needs to have HostPort 9999!")
	}
	return nil
}

func CheckBazelPortPresubmit(presubmits []Presubmit) error {
	for _, presubmit := range presubmits {
		if presubmit.Spec == nil {
			continue
		}
		hasCache := false
		for _, volume := range presubmit.Spec.Volumes {
			if volume.Name == "cache-ssd" {
				hasCache = true
			}
		}

		for _, container := range presubmit.Spec.Containers {
			if err := CheckBazelPortContainer(container, hasCache); err != nil {
				return fmt.Errorf("%s: %v", presubmit.Name, err)
			}
		}

		if err := CheckBazelPortPresubmit(presubmit.RunAfterSuccess); err != nil {
			return fmt.Errorf("%s: %v", presubmit.Name, err)
		}
	}

	return nil
}

func CheckBazelPortPostsubmit(postsubmits []Postsubmit) error {
	for _, postsubmit := range postsubmits {
		hasCache := false
		for _, volume := range postsubmit.Spec.Volumes {
			if volume.Name == "cache-ssd" {
				hasCache = true
			}
		}

		for _, container := range postsubmit.Spec.Containers {
			if err := CheckBazelPortContainer(container, hasCache); err != nil {
				return fmt.Errorf("%s: %v", postsubmit.Name, err)
			}
		}

		if err := CheckBazelPortPostsubmit(postsubmit.RunAfterSuccess); err != nil {
			return fmt.Errorf("%s: %v", postsubmit.Name, err)
		}
	}

	return nil
}

func CheckBazelPortPeriodic(periodics []Periodic) error {
	for _, periodic := range periodics {
		hasCache := false
		for _, volume := range periodic.Spec.Volumes {
			if volume.Name == "cache-ssd" {
				hasCache = true
			}
		}

		for _, container := range periodic.Spec.Containers {
			if err := CheckBazelPortContainer(container, hasCache); err != nil {
				return fmt.Errorf("%s: %v", periodic.Name, err)
			}
		}

		if err := CheckBazelPortPeriodic(periodic.RunAfterSuccess); err != nil {
			return fmt.Errorf("%s: %v", periodic.Name, err)
		}
	}

	return nil
}

// Set the HostPort to 9999 for all bazel pods so that they are forced
// onto different nodes. Once pod affinity is GA, use that instead.
// Until https://github.com/kubernetes/community/blob/master/contributors/design-proposals/local-storage-overview.md
func TestBazelJobHasContainerPort(t *testing.T) {
	c, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}

	for _, pres := range c.Presubmits {
		if err := CheckBazelPortPresubmit(pres); err != nil {
			t.Errorf("Error in presubmit: %v", err)
		}
	}

	for _, posts := range c.Postsubmits {
		if err := CheckBazelPortPostsubmit(posts); err != nil {
			t.Errorf("Error in postsubmit: %v", err)
		}
	}

	if err := CheckBazelPortPeriodic(c.Periodics); err != nil {
		t.Errorf("Error in periodic: %v", err)
	}
}
