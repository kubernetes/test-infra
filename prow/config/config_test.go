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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/ghodss/yaml"

	"k8s.io/test-infra/prow/kube"
)

// Loaded at TestMain.
var c *Config

func TestMain(m *testing.M) {
	conf, err := Load("../config.yaml")
	if err != nil {
		fmt.Printf("Could not load config: %v", err)
		os.Exit(1)
	}
	c = conf
	os.Exit(m.Run())
}

func replace(j *Presubmit, ks *Presubmit) error {
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
		if err := replace(&j.RunAfterSuccess[i], &ks.RunAfterSuccess[i]); err != nil {
			return err
		}
	}

	return nil
}

func checkContext(t *testing.T, repo string, p Presubmit) {
	if p.Name != p.Context {
		t.Errorf("Context does not match job name: %s in %s", p.Name, repo)
	}
	for _, c := range p.RunAfterSuccess {
		checkContext(t, repo, c)
	}
}

func TestContextMatches(t *testing.T) {
	for repo, presubmits := range c.Presubmits {
		for _, p := range presubmits {
			checkContext(t, repo, p)
		}
	}
}

func checkRetest(t *testing.T, repo string, presubmits []Presubmit) {
	for _, p := range presubmits {
		expected := fmt.Sprintf("/test %s", p.Name)
		if p.RerunCommand != expected {
			t.Errorf("%s in %s rerun_command: %s != expected: %s", repo, p.Name, p.RerunCommand, expected)
		}
		checkRetest(t, repo, p.RunAfterSuccess)
	}
}

func TestRetestMatchJobsName(t *testing.T) {
	for repo, presubmits := range c.Presubmits {
		checkRetest(t, repo, presubmits)
	}
}

type SubmitQueueConfig struct {
	// this is the only field we need for the tests below
	RequiredRetestContexts string `json:"required-retest-contexts"`
}

func findRequired(t *testing.T, presubmits []Presubmit) []string {
	var required []string
	for _, p := range presubmits {
		if !p.AlwaysRun {
			continue
		}
		for _, r := range findRequired(t, p.RunAfterSuccess) {
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
	b, err := ioutil.ReadFile("../../mungegithub/submit-queue/deployment/kubernetes/configmap.yaml")
	if err != nil {
		t.Fatalf("Could not load submit queue configmap: %v", err)
	}
	sqc := &SubmitQueueConfig{}
	if err = yaml.Unmarshal(b, sqc); err != nil {
		t.Fatalf("Could not parse submit queue configmap: %v", err)
	}
	required := strings.Split(sqc.RequiredRetestContexts, ",")

	running := findRequired(t, c.Presubmits["kubernetes/kubernetes"])

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
	conf, err := Load("../config.yaml")
	if err != nil {
		t.Fatalf("fail to load config for TestConfigSecurityJobsMatch : %v", err)
	}
	kp := conf.Presubmits["kubernetes/kubernetes"]
	sp := conf.Presubmits["kubernetes-security/kubernetes"]
	if len(kp) != len(sp) {
		t.Fatalf("length of kubernetes/kubernetes presubmits %d does not equal length of kubernetes-security/kubernetes presubmits %d", len(kp), len(sp))
	}
	for i, j := range kp {
		if err := replace(&j, &sp[i]); err != nil {
			t.Fatalf("[replace] : %v", err)
		}

		if !reflect.DeepEqual(j, sp[i]) {
			t.Fatalf("kubernetes/kubernetes prow config jobs do not match kubernetes-security/kubernetes jobs:\n%#v\nshould match: %#v", j, sp[i])
		}
	}
}

// checkDockerSocketVolumes returns an error if any volume uses a hostpath
// to the docker socket. we do not want to allow this
func checkDockerSocketVolumes(volumes []kube.Volume) error {
	for _, volume := range volumes {
		if volume.HostPath != nil && volume.HostPath.Path == "/var/run/docker.sock" {
			return errors.New("job uses HostPath with docker socket")
		}
	}
	return nil
}

// Make sure jobs are not using the docker socket as a host path
func TestJobDoesNotHaveDockerSocket(t *testing.T) {
	for _, pres := range c.Presubmits {
		for _, presubmit := range pres {
			if presubmit.Spec != nil {
				if err := checkDockerSocketVolumes(presubmit.Spec.Volumes); err != nil {
					t.Errorf("Error in presubmit: %v", err)
				}
			}
		}
	}

	for _, posts := range c.Postsubmits {
		for _, postsubmit := range posts {
			if postsubmit.Spec != nil {
				if err := checkDockerSocketVolumes(postsubmit.Spec.Volumes); err != nil {
					t.Errorf("Error in postsubmit: %v", err)
				}
			}
		}
	}

	for _, periodic := range c.Periodics {
		if periodic.Spec != nil {
			if err := checkDockerSocketVolumes(periodic.Spec.Volumes); err != nil {
				t.Errorf("Error in periodic: %v", err)
			}
		}
	}
}

func checkBazelPortContainer(c kube.Container, cache bool) error {
	if !cache {
		if len(c.Ports) != 0 {
			return errors.New("job does not use --cache-ssd and so should not set ports in spec")
		}
		return nil
	}

	if len(c.Ports) != 1 {
		return errors.New("job uses --cache-ssd and so needs to set ports in spec")
	} else if c.Ports[0].ContainerPort != 9999 {
		return errors.New("job uses --cache-ssd and so needs to have ContainerPort 9999")
	} else if c.Ports[0].HostPort != 9999 {
		return errors.New("job uses --cache-ssd and so needs to have HostPort 9999")
	}
	return nil
}

func checkBazelPortPresubmit(presubmits []Presubmit) error {
	for _, presubmit := range presubmits {
		if presubmit.Spec == nil {
			continue
		}
		hasCache := false
		for _, volume := range presubmit.Spec.Volumes {
			if volume.Name == "cache-ssd" || volume.Name == "docker-graph" {
				hasCache = true
			}
		}

		for _, container := range presubmit.Spec.Containers {
			if err := checkBazelPortContainer(container, hasCache); err != nil {
				return fmt.Errorf("%s: %v", presubmit.Name, err)
			}
		}

		if err := checkBazelPortPresubmit(presubmit.RunAfterSuccess); err != nil {
			return fmt.Errorf("%s: %v", presubmit.Name, err)
		}
	}

	return nil
}

func checkBazelPortPostsubmit(postsubmits []Postsubmit) error {
	for _, postsubmit := range postsubmits {
		hasCache := false
		for _, volume := range postsubmit.Spec.Volumes {
			// TODO(bentheelder): rewrite these tests and the entire caching layout...
			if volume.Name == "cache-ssd" || volume.Name == "docker-graph" {
				hasCache = true
			}
		}

		for _, container := range postsubmit.Spec.Containers {
			if err := checkBazelPortContainer(container, hasCache); err != nil {
				return fmt.Errorf("%s: %v", postsubmit.Name, err)
			}
		}

		if err := checkBazelPortPostsubmit(postsubmit.RunAfterSuccess); err != nil {
			return fmt.Errorf("%s: %v", postsubmit.Name, err)
		}
	}

	return nil
}

func checkBazelPortPeriodic(periodics []Periodic) error {
	for _, periodic := range periodics {
		hasCache := false
		for _, volume := range periodic.Spec.Volumes {
			if volume.Name == "cache-ssd" || volume.Name == "docker-graph" {
				hasCache = true
			}
		}

		for _, container := range periodic.Spec.Containers {
			if err := checkBazelPortContainer(container, hasCache); err != nil {
				return fmt.Errorf("%s: %v", periodic.Name, err)
			}
		}

		if err := checkBazelPortPeriodic(periodic.RunAfterSuccess); err != nil {
			return fmt.Errorf("%s: %v", periodic.Name, err)
		}
	}

	return nil
}

// Set the HostPort to 9999 for all bazel pods so that they are forced
// onto different nodes. Once pod affinity is GA, use that instead.
// Until https://github.com/kubernetes/community/blob/master/contributors/design-proposals/local-storage-overview.md
func TestBazelJobHasContainerPort(t *testing.T) {
	for _, pres := range c.Presubmits {
		if err := checkBazelPortPresubmit(pres); err != nil {
			t.Errorf("Error in presubmit: %v", err)
		}
	}

	for _, posts := range c.Postsubmits {
		if err := checkBazelPortPostsubmit(posts); err != nil {
			t.Errorf("Error in postsubmit: %v", err)
		}
	}

	if err := checkBazelPortPeriodic(c.Periodics); err != nil {
		t.Errorf("Error in periodic: %v", err)
	}
}

// Load the config and extract all jobs, including any child jobs inside
// RunAfterSuccess fields.
func allJobs() ([]Presubmit, []Postsubmit, []Periodic, error) {
	pres := []Presubmit{}
	posts := []Postsubmit{}
	peris := []Periodic{}

	{ // Find all presubmit jobs, including child jobs.
		q := []Presubmit{}

		for _, p := range c.Presubmits {
			for _, p2 := range p {
				q = append(q, p2)
			}
		}

		for len(q) > 0 {
			pres = append(pres, q[0])
			for _, p := range q[0].RunAfterSuccess {
				q = append(q, p)
			}
			q = q[1:]
		}
	}

	{ // Find all postsubmit jobs, including child jobs.
		q := []Postsubmit{}

		for _, p := range c.Postsubmits {
			for _, p2 := range p {
				q = append(q, p2)
			}
		}

		for len(q) > 0 {
			posts = append(posts, q[0])
			for _, p := range q[0].RunAfterSuccess {
				q = append(q, p)
			}
			q = q[1:]
		}
	}

	{ // Find all periodic jobs, including child jobs.
		q := []Periodic{}
		for _, p := range c.Periodics {
			q = append(q, p)
		}

		for len(q) > 0 {
			peris = append(peris, q[0])
			for _, p := range q[0].RunAfterSuccess {
				q = append(q, p)
			}
			q = q[1:]
		}
	}

	return pres, posts, peris, nil
}

// Validate any containers using a bazelbuild image, returning which bazelbuild tags are used.
// In particular ensure that:
//   * Presubmit, postsubmit jobs specify at least one --repo flag, the first of which uses PULL_REFS and REPO_NAME vars
//   * Prow injected vars like REPO_NAME, PULL_REFS, etc are only used on non-periodic jobs
//   * Deprecated --branch, --pull flags are not used
//   * Required --service-account, --upload, --git-cache, --job, --clean flags are present
func checkBazelbuildSpec(t *testing.T, name string, spec *kube.PodSpec, periodic bool) map[string]int {
	img := "gcr.io/k8s-testimages/bazelbuild"
	tags := map[string]int{}
	if spec == nil {
		return tags
	}
	// Tags look something like vDATE-SHA or vDATE-SHA-BAZELVERSION.
	// We want to match only on the date + sha
	tagRE := regexp.MustCompile(`^([^-]+-[^-]+)(-[^-]+)?$`)
	for _, c := range spec.Containers {
		parts := strings.SplitN(c.Image, ":", 2)
		var i, tag string // image:tag
		i = parts[0]
		if i != img {
			continue
		}
		if len(parts) == 1 {
			tag = "latest"
		} else {
			submatches := tagRE.FindStringSubmatch(parts[1])
			if submatches != nil {
				tag = submatches[1]
			} else {
				t.Errorf("bazelbuild tag '%s' doesn't match expected format", parts[1])
			}
		}
		tags[tag]++

		found := map[string][]string{}
		for _, a := range c.Args {
			parts := strings.SplitN(a, "=", 2)
			k := parts[0]
			v := "true"
			if len(parts) == 2 {
				v = parts[1]
			}
			found[k] = append(found[k], v)

			// Require --flag=FOO for easier processing
			if k == "--repo" && len(parts) == 1 {
				t.Errorf("%s: use --repo=FOO not --repo foo", name)
			}
		}

		if _, ok := found["--pull"]; ok {
			t.Errorf("%s: uses deprecated --pull arg, use --repo=org/repo=$(PULL_REFS) instead", name)
		}
		if _, ok := found["--branch"]; ok {
			t.Errorf("%s: uses deprecated --branch arg, use --repo=org/repo=$(PULL_REFS) instead", name)
		}

		for _, f := range []string{
			"--service-account",
			"--upload",
			"--git-cache",
			"--job",
			"--clean",
		} {
			if _, ok := found[f]; !ok {
				t.Errorf("%s: missing %s flag", name, f)
			}
		}

		if v, ok := found["--repo"]; !ok {
			t.Errorf("%s: missing %s flag", name, "--repo")
		} else {
			firstRepo := true
			hasRefs := false
			hasName := false
			for _, r := range v {
				hasRefs = hasRefs || strings.Contains(r, "$(PULL_REFS)")
				hasName = hasName || strings.Contains(r, "$(REPO_NAME)")
				if !firstRepo {
					t.Errorf("%s: has too many --repo. REMOVE THIS CHECK BEFORE MERGE", name)
				}
				for _, d := range []string{
					"$(REPO_NAME)",
					"$(REPO_OWNER)",
					"$(PULL_BASE_REF)",
					"$(PULL_BASE_SHA)",
					"$(PULL_REFS)",
					"$(PULL_NUMBER)",
					"$(PULL_PULL_SHA)",
				} {
					has := strings.Contains(r, d)
					if periodic && has {
						t.Errorf("%s: %s are not available to periodic jobs, please use a static --repo=org/repo=branch", name, d)
					} else if !firstRepo && has {
						t.Errorf("%s: %s are only relevant to the first --repo flag, remove from --repo=%s", name, d, r)
					}
				}
				firstRepo = false
			}
			if !periodic && !hasRefs {
				t.Errorf("%s: non-periodic jobs need a --repo=org/branch=$(PULL_REFS) somewhere", name)
			}
			if !periodic && !hasName {
				t.Errorf("%s: non-periodic jobs need a --repo=org/$(REPO_NAME) somewhere", name)
			}
		}

		if c.Resources.Requests == nil {
			t.Errorf("%s: bazel jobs need to place a resource request", name)
		}
	}
	return tags
}

// Unit test jobs that use a bazelbuild image do so correctly.
func TestBazelbuildArgs(t *testing.T) {
	pres, posts, peris, err := allJobs()
	if err != nil {
		t.Fatalf("Could not load config: %v", err)
	}

	tags := map[string][]string{} // tag -> jobs map
	for _, p := range pres {
		for t := range checkBazelbuildSpec(t, p.Name, p.Spec, false) {
			tags[t] = append(tags[t], p.Name)
		}
	}
	for _, p := range posts {
		for t := range checkBazelbuildSpec(t, p.Name, p.Spec, false) {
			tags[t] = append(tags[t], p.Name)
		}
	}
	for _, p := range peris {
		for t := range checkBazelbuildSpec(t, p.Name, p.Spec, true) {
			tags[t] = append(tags[t], p.Name)
		}
	}
	pinnedJobs := map[string]string{
		//job: reason for pinning
		// these frequently need to be pinned...
		//"pull-test-infra-bazel":              "test-infra adopts bazel upgrades first",
		//"ci-test-infra-bazel":                "test-infra adopts bazel upgrades first",
		"pull-test-infra-bazel-canary":       "canary testing the latest bazel",
		"pull-kubernetes-bazel-build-canary": "canary testing the latest bazel",
		"pull-kubernetes-bazel-test-canary":  "canary testing the latest bazel",
	}
	// auto insert pull-security-kubernetes-*
	for job, reason := range pinnedJobs {
		if strings.HasPrefix(job, "pull-kubernetes") {
			pinnedJobs[strings.Replace(job, "pull-kubernetes", "pull-security-kubernetes", 1)] = reason
		}
	}
	maxTag := ""
	maxN := 0
	for t, js := range tags {
		n := len(js)
		if n > maxN {
			maxTag = t
			maxN = n
		}
	}
	for tag, js := range tags {
		current := tag == maxTag
		for _, j := range js {
			if v, pinned := pinnedJobs[j]; !pinned && !current {
				t.Errorf("%s: please add to the pinnedJobs list or else update tag to %s", j, maxTag)
			} else if current && pinned {
				t.Errorf("%s: please remove from the pinnedJobs list", j)
			} else if !current && v == "" {
				t.Errorf("%s: pinning to a non-default version requires a non-empty reason for doing so", j)
			}
		}
	}
}

func TestURLTemplate(t *testing.T) {
	testcases := []struct {
		name    string
		jobType kube.ProwJobType
		org     string
		repo    string
		job     string
		build   string
		expect  string
	}{
		{
			name:    "k8s presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/0/k8s-pre-1/1/",
		},
		{
			name:    "k8s/test-infra presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes",
			repo:    "test-infra",
			job:     "ti-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/test-infra/0/ti-pre-1/1/",
		},
		{
			name:    "foo/k8s presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/foo_kubernetes/0/k8s-pre-1/1/",
		},
		{
			name:    "foo-bar presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "bar",
			job:     "foo-pre-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/foo_bar/0/foo-pre-1/1/",
		},
		{
			name:    "k8s postsubmit",
			jobType: kube.PostsubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-post-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/k8s-post-1/1/",
		},
		{
			name:    "k8s periodic",
			jobType: kube.PeriodicJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-peri-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/k8s-peri-1/1/",
		},
		{
			name:    "empty periodic",
			jobType: kube.PeriodicJob,
			org:     "",
			repo:    "",
			job:     "nan-peri-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/logs/nan-peri-1/1/",
		},
		{
			name:    "k8s batch",
			jobType: kube.BatchJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-batch-1",
			build:   "1",
			expect:  "https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/batch/k8s-batch-1/1/",
		},
	}

	for _, tc := range testcases {
		var pj = kube.ProwJob{
			Metadata: kube.ObjectMeta{Name: tc.name},
			Spec: kube.ProwJobSpec{
				Type: tc.jobType,
				Job:  tc.job,
				Refs: kube.Refs{
					Pulls: []kube.Pull{{}},
					Org:   tc.org,
					Repo:  tc.repo,
				},
			},
			Status: kube.ProwJobStatus{
				BuildID: tc.build,
			},
		}

		var b bytes.Buffer
		if err := c.Plank.JobURLTemplate.Execute(&b, &pj); err != nil {
			t.Fatalf("Error executing template: %v", err)
		}
		res := b.String()
		if res != tc.expect {
			t.Errorf("tc: %s, Expect URL: %s, got %s", tc.name, tc.expect, res)
		}
	}
}

func TestReportTemplate(t *testing.T) {
	var testcases = []struct {
		org    string
		repo   string
		number int
		suffix string
	}{
		{
			org:    "o",
			repo:   "r",
			number: 4,
			suffix: "o_r/4",
		},
		{
			org:    "kubernetes",
			repo:   "test-infra",
			number: 123,
			suffix: "test-infra/123",
		},
		{
			org:    "kubernetes",
			repo:   "kubernetes",
			number: 123,
			suffix: "123",
		},
		{
			org:    "o",
			repo:   "kubernetes",
			number: 456,
			suffix: "o_kubernetes/456",
		},
	}
	for _, tc := range testcases {
		var b bytes.Buffer
		if err := c.Plank.ReportTemplate.Execute(&b, &kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Refs: kube.Refs{
					Org:  tc.org,
					Repo: tc.repo,
					Pulls: []kube.Pull{
						{
							Number: tc.number,
						},
					},
				},
			},
		}); err != nil {
			t.Errorf("Error executing template: %v", err)
			continue
		}
		expectedPath := "https://k8s-gubernator.appspot.com/pr/" + tc.suffix
		if !strings.Contains(b.String(), expectedPath) {
			t.Errorf("Expected template to contain %s, but it didn't: %s", expectedPath, b.String())
		}
	}
}

func TestPullKubernetesCross(t *testing.T) {
	crossBuildJob := "pull-kubernetes-cross"
	tests := []struct {
		changedFile string
		expected    bool
	}{
		{
			changedFile: "pkg/kubelet/cadvisor/cadvisor_unsupported.go",
			expected:    true,
		},
		{
			changedFile: "pkg/kubelet/cadvisor/util.go",
			expected:    false,
		},
		{
			changedFile: "Makefile",
			expected:    true,
		},
		{
			changedFile: "hack/lib/etcd.sh",
			expected:    true,
		},
		{
			changedFile: "build/debs/kubelet.service",
			expected:    true,
		},
		{
			changedFile: "federation/README.md",
			expected:    false,
		},
	}
	kkPresumits := c.Presubmits["kubernetes/kubernetes"]
	var cross *Presubmit
	for i := range kkPresumits {
		ps := kkPresumits[i]
		if ps.Name == crossBuildJob {
			cross = &ps
			break
		}
	}
	if cross == nil {
		t.Fatalf("expected %q in the presubmit section of the prow config", crossBuildJob)
	}

	for i, test := range tests {
		t.Logf("test run #%d", i)
		got := cross.RunsAgainstChanges([]string{test.changedFile})
		if got != test.expected {
			t.Errorf("expected changes (%s) to run cross job: %t, got: %t",
				test.changedFile, test.expected, got)
		}
	}
}

// checkLatestUsesImagePullPolicy returns an error if an image is a `latest-.*` tag,
// but doesn't have imagePullPolicy: Always
func checkLatestUsesImagePullPolicy(spec *kube.PodSpec) error {
	for _, container := range spec.Containers {
		if strings.Contains(container.Image, ":latest-") {
			// If the job doesn't specify imagePullPolicy: Always,
			// we aren't guaranteed to check for the latest version of the image.
			if container.ImagePullPolicy == "" || container.ImagePullPolicy != "Always" {
				return errors.New("job uses latest- tag, but does not specify imagePullPolicy: Always")
			}
		}
		if strings.HasSuffix(container.Image, ":latest") {
			// The k8s default for `:latest` images is `imagePullPolicy: Always`
			// Check the job didn't override
			if container.ImagePullPolicy != "" && container.ImagePullPolicy != "Always" {
				return errors.New("job uses latest tag, but does not specify imagePullPolicy: Always")
			}
		}

	}
	return nil
}

// Make sure jobs that use `latest-*` tags specify `imagePullPolicy: Always`
func TestLatestUsesImagePullPolicy(t *testing.T) {
	for _, pres := range c.Presubmits {
		for _, presubmit := range pres {
			if presubmit.Spec != nil {
				if err := checkLatestUsesImagePullPolicy(presubmit.Spec); err != nil {
					t.Errorf("Error in presubmit %q: %v", presubmit.Name, err)
				}
			}
		}
	}

	for _, posts := range c.Postsubmits {
		for _, postsubmit := range posts {
			if postsubmit.Spec != nil {
				if err := checkLatestUsesImagePullPolicy(postsubmit.Spec); err != nil {
					t.Errorf("Error in postsubmit %q: %v", postsubmit.Name, err)
				}
			}
		}
	}

	for _, periodic := range c.Periodics {
		if periodic.Spec != nil {
			if err := checkLatestUsesImagePullPolicy(periodic.Spec); err != nil {
				t.Errorf("Error in periodic %q: %v", periodic.Name, err)
			}
		}
	}
}
