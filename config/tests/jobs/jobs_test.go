/*
Copyright 2018 The Kubernetes Authors.

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

package tests

// This file validates kubernetes's jobs configs.
// See also prow/config/jobstests for generic job tests that
// all deployments should consider using.

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cfg "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

var configPath = flag.String("config", "../../../prow/config.yaml", "Path to prow config")
var jobConfigPath = flag.String("job-config", "../../jobs", "Path to prow job config")
var gubernatorPath = flag.String("gubernator-path", "https://gubernator.k8s.io", "Path to linked gubernator")
var bucket = flag.String("bucket", "kubernetes-jenkins", "Gcs bucket for log upload")
var k8sProw = flag.Bool("k8s-prow", true, "If the config is for k8s prow cluster")

// Loaded at TestMain.
var c *cfg.Config

func TestMain(m *testing.M) {
	flag.Parse()
	if *configPath == "" {
		fmt.Println("--config must set")
		os.Exit(1)
	}

	conf, err := cfg.Load(*configPath, *jobConfigPath)
	if err != nil {
		fmt.Printf("Could not load config: %v", err)
		os.Exit(1)
	}
	c = conf

	os.Exit(m.Run())
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
				Refs: &kube.Refs{
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
		expectedPath := *gubernatorPath + "/pr/" + tc.suffix
		if !strings.Contains(b.String(), expectedPath) {
			t.Errorf("Expected template to contain %s, but it didn't: %s", expectedPath, b.String())
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
		k8sOnly bool
	}{
		{
			name:    "k8s presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/pr-logs/pull/0/k8s-pre-1/1/",
			k8sOnly: true,
		},
		{
			name:    "k8s-security presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes-security",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  "https://console.cloud.google.com/storage/browser/kubernetes-security-prow/pr-logs/pull/kubernetes-security_kubernetes/0/k8s-pre-1/1/",
			k8sOnly: true,
		},
		{
			name:    "k8s/test-infra presubmit",
			jobType: kube.PresubmitJob,
			org:     "kubernetes",
			repo:    "test-infra",
			job:     "ti-pre-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/pr-logs/pull/test-infra/0/ti-pre-1/1/",
			k8sOnly: true,
		},
		{
			name:    "foo/k8s presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/pr-logs/pull/foo_kubernetes/0/k8s-pre-1/1/",
		},
		{
			name:    "foo-bar presubmit",
			jobType: kube.PresubmitJob,
			org:     "foo",
			repo:    "bar",
			job:     "foo-pre-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/pr-logs/pull/foo_bar/0/foo-pre-1/1/",
		},
		{
			name:    "k8s postsubmit",
			jobType: kube.PostsubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-post-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/logs/k8s-post-1/1/",
		},
		{
			name:    "k8s periodic",
			jobType: kube.PeriodicJob,
			job:     "k8s-peri-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/logs/k8s-peri-1/1/",
		},
		{
			name:    "empty periodic",
			jobType: kube.PeriodicJob,
			job:     "nan-peri-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/logs/nan-peri-1/1/",
		},
		{
			name:    "k8s batch",
			jobType: kube.BatchJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-batch-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/pr-logs/pull/batch/k8s-batch-1/1/",
			k8sOnly: true,
		},
		{
			name:    "foo bar batch",
			jobType: kube.BatchJob,
			org:     "foo",
			repo:    "bar",
			job:     "k8s-batch-1",
			build:   "1",
			expect:  *gubernatorPath + "/build/" + *bucket + "/pr-logs/pull/foo_bar/batch/k8s-batch-1/1/",
		},
	}

	for _, tc := range testcases {
		if !*k8sProw && tc.k8sOnly {
			continue
		}

		var pj = kube.ProwJob{
			ObjectMeta: metav1.ObjectMeta{Name: tc.name},
			Spec: kube.ProwJobSpec{
				Type: tc.jobType,
				Job:  tc.job,
			},
			Status: kube.ProwJobStatus{
				BuildID: tc.build,
			},
		}
		if tc.jobType != kube.PeriodicJob {
			pj.Spec.Refs = &kube.Refs{
				Pulls: []kube.Pull{{}},
				Org:   tc.org,
				Repo:  tc.repo,
			}
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

func checkContext(t *testing.T, repo string, p cfg.Presubmit) {
	if !p.SkipReport && p.Name != p.Context {
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

func checkRetest(t *testing.T, repo string, presubmits []cfg.Presubmit) {
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

func findRequired(t *testing.T, presubmits []cfg.Presubmit) []string {
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

func TestTrustedJobs(t *testing.T) {
	// TODO(fejta): allow each config/jobs/kubernetes/foo/foo-trusted.yaml
	// that uses a foo-trusted cluster
	const trusted = "test-infra-trusted"
	trustedPath := path.Join(*jobConfigPath, "kubernetes", "test-infra", "test-infra-trusted.yaml")

	// Presubmits may not use trusted clusters.
	for _, pre := range c.AllPresubmits(nil) {
		if pre.Cluster == trusted {
			t.Errorf("%s: presubmits cannot use trusted clusters", pre.Name)
		}
	}

	// Trusted postsubmits must be defined in trustedPath
	for _, post := range c.AllPostsubmits(nil) {
		if post.Cluster != trusted {
			continue
		}
		if post.SourcePath != trustedPath {
			t.Errorf("%s defined in %s may not run in trusted cluster", post.Name, post.SourcePath)
		}
	}

	// Trusted periodics must be defined in trustedPath
	for _, per := range c.AllPeriodics() {
		if per.Cluster != trusted {
			continue
		}
		if per.SourcePath != trustedPath {
			t.Errorf("%s defined in %s may not run in trusted cluster", per.Name, per.SourcePath)
		}
	}
}

// Unit test jobs outside kubernetes-security do not use the security cluster
// and that jobs inside kubernetes-security DO
func TestConfigSecurityClusterRestricted(t *testing.T) {
	for repo, jobs := range c.Presubmits {
		if strings.HasPrefix(repo, "kubernetes-security/") {
			for _, job := range jobs {
				if job.Agent != "jenkins" && job.Cluster != "security" {
					t.Fatalf("Jobs in kubernetes-security/* should use the security cluster! %s", job.Name)
				}
			}
		} else {
			for _, job := range jobs {
				if job.Cluster == "security" {
					t.Fatalf("Jobs not in kubernetes-security/* should not use the security cluster! %s", job.Name)
				}
			}
		}
	}
	for repo, jobs := range c.Postsubmits {
		if strings.HasPrefix(repo, "kubernetes-security/") {
			for _, job := range jobs {
				if job.Agent != "jenkins" && job.Cluster != "security" {
					t.Fatalf("Jobs in kubernetes-security/* should use the security cluster! %s", job.Name)
				}
			}
		} else {
			for _, job := range jobs {
				if job.Cluster == "security" {
					t.Fatalf("Jobs not in kubernetes-security/* should not use the security cluster! %s", job.Name)
				}
			}
		}
	}
	// TODO(bentheelder): this will need to be more complex if we ever add k-s periodic
	for _, job := range c.AllPeriodics() {
		if job.Cluster == "security" {
			t.Fatalf("Jobs not in kubernetes-security/* should not use the security cluster! %s", job.Name)
		}
	}
}

// checkDockerSocketVolumes returns an error if any volume uses a hostpath
// to the docker socket. we do not want to allow this
func checkDockerSocketVolumes(volumes []v1.Volume) error {
	for _, volume := range volumes {
		if volume.HostPath != nil && volume.HostPath.Path == "/var/run/docker.sock" {
			return errors.New("job uses HostPath with docker socket")
		}
	}
	return nil
}

// Make sure jobs are not using the docker socket as a host path
func TestJobDoesNotHaveDockerSocket(t *testing.T) {
	for _, presubmit := range c.AllPresubmits(nil) {
		if presubmit.Spec != nil {
			if err := checkDockerSocketVolumes(presubmit.Spec.Volumes); err != nil {
				t.Errorf("Error in presubmit: %v", err)
			}
		}
	}

	for _, postsubmit := range c.AllPostsubmits(nil) {
		if postsubmit.Spec != nil {
			if err := checkDockerSocketVolumes(postsubmit.Spec.Volumes); err != nil {
				t.Errorf("Error in postsubmit: %v", err)
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

// checkLatestUsesImagePullPolicy returns an error if an image is a `latest-.*` tag,
// but doesn't have imagePullPolicy: Always
func checkLatestUsesImagePullPolicy(spec *v1.PodSpec) error {
	for _, container := range spec.Containers {
		if strings.Contains(container.Image, ":latest-") {
			// If the job doesn't specify imagePullPolicy: Always,
			// we aren't guaranteed to check for the latest version of the image.
			if container.ImagePullPolicy != "Always" {
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
	for _, presubmit := range c.AllPresubmits(nil) {
		if presubmit.Spec != nil {
			if err := checkLatestUsesImagePullPolicy(presubmit.Spec); err != nil {
				t.Errorf("Error in presubmit %q: %v", presubmit.Name, err)
			}
		}
	}

	for _, postsubmit := range c.AllPostsubmits(nil) {
		if postsubmit.Spec != nil {
			if err := checkLatestUsesImagePullPolicy(postsubmit.Spec); err != nil {
				t.Errorf("Error in postsubmit %q: %v", postsubmit.Name, err)
			}
		}
	}

	for _, periodic := range c.AllPeriodics() {
		if periodic.Spec != nil {
			if err := checkLatestUsesImagePullPolicy(periodic.Spec); err != nil {
				t.Errorf("Error in periodic %q: %v", periodic.Name, err)
			}
		}
	}
}

// checkKubekinsPresets returns an error if a spec references to kubekins-e2e|bootstrap image,
// but doesn't use service preset or ssh preset
func checkKubekinsPresets(jobName string, spec *v1.PodSpec, labels, validLabels map[string]string) error {
	service := true
	ssh := true

	for _, container := range spec.Containers {
		if strings.Contains(container.Image, "kubekins-e2e") || strings.Contains(container.Image, "bootstrap") {
			service = false
			for key, val := range labels {
				if (key == "preset-gke-alpha-service" || key == "preset-service-account" || key == "preset-istio-service") && val == "true" {
					service = true
				}
			}
		}

		scenario := ""
		for _, arg := range container.Args {
			if strings.HasPrefix(arg, "--scenario=") {
				scenario = strings.TrimPrefix(arg, "--scenario=")
			}
		}

		if scenario == "kubenetes_e2e" {
			ssh = false
			for key, val := range labels {
				if (key == "preset-k8s-ssh" || key == "preset-aws-ssh") && val == "true" {
					ssh = true
				}
			}
		}
	}

	if !service {
		return fmt.Errorf("cannot find service account preset")
	}

	if !ssh {
		return fmt.Errorf("cannot find ssh preset")
	}

	for key, val := range labels {
		if validVal, ok := validLabels[key]; !ok {
			return fmt.Errorf("label %s is not a valid preset label", key)
		} else if validVal != val {
			return fmt.Errorf("label %s does not have valid value, have %s, expect %s", key, val, validVal)
		}
	}

	return nil
}

// TestValidPresets makes sure all presets name starts with 'preset-', all job presets are valid,
// and jobs that uses kubekins-e2e image has the right service account preset
func TestValidPresets(t *testing.T) {
	validLabels := map[string]string{}
	for _, preset := range c.Presets {
		for label, val := range preset.Labels {
			if !strings.HasPrefix(label, "preset-") {
				t.Errorf("Preset label %s - label name should start with 'preset-'", label)
			} else if val != "true" {
				t.Errorf("Preset label %s - label value should be true", label)
			}
			if _, ok := validLabels[label]; ok {
				t.Errorf("Duplicated preset label : %s", label)
			} else {
				validLabels[label] = val
			}
		}
	}

	if !*k8sProw {
		return
	}

	for _, presubmit := range c.AllPresubmits(nil) {
		if presubmit.Spec != nil && !presubmit.Decorate {
			if err := checkKubekinsPresets(presubmit.Name, presubmit.Spec, presubmit.Labels, validLabels); err != nil {
				t.Errorf("Error in presubmit %q: %v", presubmit.Name, err)
			}
		}
	}

	for _, postsubmit := range c.AllPostsubmits(nil) {
		if postsubmit.Spec != nil && !postsubmit.Decorate {
			if err := checkKubekinsPresets(postsubmit.Name, postsubmit.Spec, postsubmit.Labels, validLabels); err != nil {
				t.Errorf("Error in postsubmit %q: %v", postsubmit.Name, err)
			}
		}
	}

	for _, periodic := range c.AllPeriodics() {
		if periodic.Spec != nil && !periodic.Decorate {
			if err := checkKubekinsPresets(periodic.Name, periodic.Spec, periodic.Labels, validLabels); err != nil {
				t.Errorf("Error in periodic %q: %v", periodic.Name, err)
			}
		}
	}
}

func hasArg(wanted string, args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, wanted) {
			return true
		}
	}

	return false
}

func checkScenarioArgs(jobName, imageName string, args []string) error {
	// env files/scenarios validation
	scenarioArgs := false
	scenario := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "--env-file=") {
			return fmt.Errorf("job %s: --env-file is deprecated, please migrate to presets %s", jobName, arg)
		}

		if arg == "--" {
			scenarioArgs = true
		}

		if strings.HasPrefix(arg, "--scenario=") {
			scenario = strings.TrimPrefix(arg, "--scenario=")
		}
	}

	if scenario == "" {
		entry := jobName
		if strings.HasPrefix(jobName, "pull-security-kubernetes") {
			entry = strings.Replace(entry, "pull-security-kubernetes", "pull-kubernetes", -1)
		}

		if !scenarioArgs {
			if strings.Contains(imageName, "kubekins-e2e") ||
				strings.Contains(imageName, "bootstrap") ||
				strings.Contains(imageName, "gcloud-in-go") {
				return fmt.Errorf("job %s: image %s uses bootstrap.py and need scenario args", jobName, imageName)
			}
			return nil
		}

	} else {
		if _, err := os.Stat(fmt.Sprintf("../../../scenarios/%s.py", scenario)); err != nil {
			return fmt.Errorf("job %s: scenario %s does not exist: %s", jobName, scenario, err)
		}

		if !scenarioArgs {
			return fmt.Errorf("job %s: set --scenario=%s and will need scenario args", jobName, scenario)
		}
	}

	// shared build args
	use_shared_build_in_args := hasArg("--use-shared-build", args)
	extract_in_args := hasArg("--extract", args)
	build_in_args := hasArg("--build", args)

	if use_shared_build_in_args && extract_in_args {
		return fmt.Errorf("job %s: --use-shared-build and --extract cannot be combined", jobName)
	}

	if use_shared_build_in_args && build_in_args {
		return fmt.Errorf("job %s: --use-shared-build and --build cannot be combined", jobName)
	}

	if scenario != "kubernetes_e2e" {
		return nil
	}

	if hasArg("--provider=gke", args) {
		if !hasArg("--deployment=gke", args) {
			return fmt.Errorf("with --provider=gke, job %s must use --deployment=gke", jobName)
		}
		if hasArg("--gcp-master-image", args) {
			return fmt.Errorf("with --provider=gke, job %s cannot use --gcp-master-image", jobName)
		}
		if hasArg("--gcp-nodes", args) {
			return fmt.Errorf("with --provider=gke, job %s cannot use --gcp-nodes", jobName)
		}
	}

	if hasArg("--deployment=gke", args) && !hasArg("--gcp-node-image", args) {
		return fmt.Errorf("with --deployment=gke, job %s must use --gcp-node-image", jobName)
	}

	if hasArg("--stage=gs://kubernetes-release-pull", args) && hasArg("--check-leaked-resources", args) {
		return fmt.Errorf("presubmit job %s should not check for resource leaks", jobName)
	}

	extracts := hasArg("--extract=", args)
	sharedBuilds := hasArg("--use-shared-build", args)
	nodeE2e := hasArg("--deployment=node", args)
	localE2e := hasArg("--deployment=local", args)
	builds := hasArg("--build", args)

	if sharedBuilds && extracts {
		return fmt.Errorf("e2e jobs %s cannot have --use-shared-build and --extract", jobName)
	}

	if !sharedBuilds && !extracts && !nodeE2e && !builds {
		return fmt.Errorf("e2e jobs %s should get k8s build from one of --extract, --use-shared-build, --build or use --deployment=node", jobName)
	}

	expectedExtract := 1
	if sharedBuilds || nodeE2e {
		expectedExtract = 0
	} else if builds && !extracts {
		expectedExtract = 0
	} else if strings.Contains(jobName, "ingress") {
		expectedExtract = 1
	} else if strings.Contains(jobName, "upgrade") ||
		strings.Contains(jobName, "skew") ||
		strings.Contains(jobName, "rollback") ||
		strings.Contains(jobName, "downgrade") ||
		jobName == "ci-kubernetes-e2e-gce-canary" {
		expectedExtract = 2
	}

	numExtract := 0
	for _, arg := range args {
		if strings.HasPrefix(arg, "--extract=") {
			numExtract++
		}
	}
	if numExtract != expectedExtract {
		return fmt.Errorf("e2e jobs %s should have %d --extract flags, got %d", jobName, expectedExtract, numExtract)
	}

	if hasArg("--image-family", args) != hasArg("--image-project", args) {
		return fmt.Errorf("e2e jobs %s should have both --image-family and --image-project, or none of them", jobName)
	}

	if strings.HasPrefix(jobName, "pull-kubernetes-") &&
		!nodeE2e &&
		!localE2e &&
		!strings.Contains(jobName, "kubeadm") {
		stage := "gs://kubernetes-release-pull/ci/" + jobName
		if strings.Contains(jobName, "gke") {
			stage = "gs://kubernetes-release-dev/ci"
			if !hasArg("--stage-suffix="+jobName, args) {
				return fmt.Errorf("presubmit gke jobs %s - need to have --stage-suffix=%s", jobName, jobName)
			}
		}

		if !sharedBuilds {
			if !hasArg("--stage="+stage, args) {
				return fmt.Errorf("presubmit jobs %s - need to stage to %s", jobName, stage)
			}
		}
	}

	// test_args should not have double slashes on ginkgo flags
	for _, arg := range args {
		ginkgo_args := ""
		if strings.HasPrefix(arg, "--test_args=") {
			split := strings.SplitN(arg, "=", 2)
			ginkgo_args = split[1]
		} else if strings.HasPrefix(arg, "--upgrade_args=") {
			split := strings.SplitN(arg, "=", 2)
			ginkgo_args = split[1]
		}

		if strings.Contains(ginkgo_args, "\\\\") {
			return fmt.Errorf("jobs %s - double slashes in ginkgo args should be single slash now : arg %s", jobName, arg)
		}
	}

	// timeout should be valid
	bootstrap_timeout := 0 * time.Minute
	kubetest_timeout := 0 * time.Minute
	var err error
	kubetest := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--timeout=") {
			timeout := strings.SplitN(arg, "=", 2)[1]
			if kubetest {
				if kubetest_timeout, err = time.ParseDuration(timeout); err != nil {
					return fmt.Errorf("jobs %s - invalid kubetest timeout : arg %s", jobName, arg)
				}
			} else {
				if bootstrap_timeout, err = time.ParseDuration(timeout + "m"); err != nil {
					return fmt.Errorf("jobs %s - invalid bootstrap timeout : arg %s", jobName, arg)
				}
			}
		}

		if arg == "--" {
			kubetest = true
		}
	}

	if bootstrap_timeout.Minutes()-kubetest_timeout.Minutes() < 20.0 {
		return fmt.Errorf(
			"jobs %s - kubetest timeout(%v), bootstrap timeout(%v): bootstrap timeout need to be 20min more than kubetest timeout!", jobName, kubetest_timeout, bootstrap_timeout)
	}

	return nil
}

// TestValidScenarioArgs makes sure all scenario args in job configs are valid
func TestValidScenarioArgs(t *testing.T) {
	for _, job := range c.AllPresubmits(nil) {
		if job.Spec != nil && !job.Decorate {
			if err := checkScenarioArgs(job.Name, job.Spec.Containers[0].Image, job.Spec.Containers[0].Args); err != nil {
				t.Errorf("Invalid Scenario Args : %s", err)
			}
		}
	}

	for _, job := range c.AllPostsubmits(nil) {
		if job.Spec != nil && !job.Decorate {
			if err := checkScenarioArgs(job.Name, job.Spec.Containers[0].Image, job.Spec.Containers[0].Args); err != nil {
				t.Errorf("Invalid Scenario Args : %s", err)
			}
		}
	}

	for _, job := range c.AllPeriodics() {
		if job.Spec != nil && !job.Decorate {
			if err := checkScenarioArgs(job.Name, job.Spec.Containers[0].Image, job.Spec.Containers[0].Args); err != nil {
				t.Errorf("Invalid Scenario Args : %s", err)
			}
		}
	}
}
