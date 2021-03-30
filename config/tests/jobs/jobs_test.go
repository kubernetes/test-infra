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
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	cfg "k8s.io/test-infra/prow/config"
)

var configPath = flag.String("config", "../../../config/prow/config.yaml", "Path to prow config")
var jobConfigPath = flag.String("job-config", "../../jobs", "Path to prow job config")
var deckPath = flag.String("deck-path", "https://prow.k8s.io", "Path to deck")
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

	conf, err := cfg.Load(*configPath, *jobConfigPath, nil)
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
			suffix: "?org=o&repo=r&pr=4",
		},
		{
			org:    "kubernetes",
			repo:   "test-infra",
			number: 123,
			suffix: "?org=kubernetes&repo=test-infra&pr=123",
		},
		{
			org:    "kubernetes",
			repo:   "kubernetes",
			number: 123,
			suffix: "?org=kubernetes&repo=kubernetes&pr=123",
		},
		{
			org:    "o",
			repo:   "kubernetes",
			number: 456,
			suffix: "?org=o&repo=kubernetes&pr=456",
		},
	}
	for _, tc := range testcases {
		var b bytes.Buffer
		refs := &prowapi.Refs{
			Org:  tc.org,
			Repo: tc.repo,
			Pulls: []prowapi.Pull{
				{
					Number: tc.number,
				},
			},
		}

		reportTemplate := c.Plank.ReportTemplateForRepo(refs)
		if err := reportTemplate.Execute(&b, &prowapi.ProwJob{Spec: prowapi.ProwJobSpec{Refs: refs}}); err != nil {
			t.Errorf("Error executing template: %v", err)
			continue
		}
		expectedPath := *deckPath + "/pr-history" + tc.suffix
		if !strings.Contains(b.String(), expectedPath) {
			t.Errorf("Expected template to contain %s, but it didn't: %s", expectedPath, b.String())
		}
	}
}

func TestURLTemplate(t *testing.T) {
	testcases := []struct {
		name    string
		jobType prowapi.ProwJobType
		org     string
		repo    string
		job     string
		build   string
		expect  string
		k8sOnly bool
	}{
		{
			name:    "k8s presubmit",
			jobType: prowapi.PresubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/pr-logs/pull/0/k8s-pre-1/1/",
			k8sOnly: true,
		},
		{
			name:    "k8s/test-infra presubmit",
			jobType: prowapi.PresubmitJob,
			org:     "kubernetes",
			repo:    "test-infra",
			job:     "ti-pre-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/pr-logs/pull/test-infra/0/ti-pre-1/1/",
			k8sOnly: true,
		},
		{
			name:    "foo/k8s presubmit",
			jobType: prowapi.PresubmitJob,
			org:     "foo",
			repo:    "kubernetes",
			job:     "k8s-pre-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/pr-logs/pull/foo_kubernetes/0/k8s-pre-1/1/",
		},
		{
			name:    "foo-bar presubmit",
			jobType: prowapi.PresubmitJob,
			org:     "foo",
			repo:    "bar",
			job:     "foo-pre-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/pr-logs/pull/foo_bar/0/foo-pre-1/1/",
		},
		{
			name:    "k8s postsubmit",
			jobType: prowapi.PostsubmitJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-post-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/logs/k8s-post-1/1/",
		},
		{
			name:    "k8s periodic",
			jobType: prowapi.PeriodicJob,
			job:     "k8s-peri-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/logs/k8s-peri-1/1/",
		},
		{
			name:    "empty periodic",
			jobType: prowapi.PeriodicJob,
			job:     "nan-peri-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/logs/nan-peri-1/1/",
		},
		{
			name:    "k8s batch",
			jobType: prowapi.BatchJob,
			org:     "kubernetes",
			repo:    "kubernetes",
			job:     "k8s-batch-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/pr-logs/pull/batch/k8s-batch-1/1/",
			k8sOnly: true,
		},
		{
			name:    "foo bar batch",
			jobType: prowapi.BatchJob,
			org:     "foo",
			repo:    "bar",
			job:     "k8s-batch-1",
			build:   "1",
			expect:  *deckPath + "/view/gs/" + *bucket + "/pr-logs/pull/foo_bar/batch/k8s-batch-1/1/",
		},
	}

	for _, tc := range testcases {
		if !*k8sProw && tc.k8sOnly {
			continue
		}

		var pj = prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{Name: tc.name},
			Spec: prowapi.ProwJobSpec{
				Type: tc.jobType,
				Job:  tc.job,
			},
			Status: prowapi.ProwJobStatus{
				BuildID: tc.build,
			},
		}
		if tc.jobType != prowapi.PeriodicJob {
			pj.Spec.Refs = &prowapi.Refs{
				Pulls: []prowapi.Pull{{}},
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
}

func TestContextMatches(t *testing.T) {
	for repo, presubmits := range c.PresubmitsStatic {
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
	}
}

func TestRetestMatchJobsName(t *testing.T) {
	for repo, presubmits := range c.PresubmitsStatic {
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
		if p.SkipReport {
			continue
		}
		required = append(required, p.Context)
	}
	return required
}

// Enforce conventions for jobs that run in test-infra-trusted cluster
func TestTrustedJobs(t *testing.T) {
	// TODO(fejta): allow each config/jobs/kubernetes/foo/foo-trusted.yaml
	// that uses a foo-trusted cluster
	const trusted = "test-infra-trusted"
	trustedPath := path.Join(*jobConfigPath, "kubernetes", "test-infra", "test-infra-trusted.yaml")

	// Presubmits may not use trusted clusters.
	for _, pre := range c.AllStaticPresubmits(nil) {
		if pre.Cluster == trusted {
			t.Errorf("%s: presubmits cannot use trusted clusters", pre.Name)
		}
	}

	// Trusted postsubmits must be defined in trustedPath
	for _, post := range c.AllStaticPostsubmits(nil) {
		if post.Cluster == trusted && post.SourcePath != trustedPath {
			t.Errorf("%s defined in %s may not run in trusted cluster", post.Name, post.SourcePath)
		}
	}

	// Trusted periodics must be defined in trustedPath
	for _, per := range c.AllPeriodics() {
		if per.Cluster == trusted && per.SourcePath != trustedPath {
			t.Errorf("%s defined in %s may not run in trusted cluster", per.Name, per.SourcePath)
		}
	}
}

// Enforce conventions for jobs that run in k8s-infra-prow-build-trusted cluster
func TestK8sInfraTrusted(t *testing.T) {
	const trusted = "k8s-infra-prow-build-trusted"
	trustedPath := path.Join(*jobConfigPath, "kubernetes", "wg-k8s-infra", "trusted") + "/"
	imagePushingDir := path.Join(*jobConfigPath, "image-pushing") + "/"

	// Presubmits may not use this cluster
	for _, pre := range c.AllStaticPresubmits(nil) {
		if pre.Cluster == trusted {
			t.Errorf("%s: presubmits may not run in cluster: %s", pre.Name, trusted)
		}
	}

	// Postsubmits and periodics must
	// - be defined in config/jobs/image-pushing/ and be a valid image-pushing job, OR
	// - be defined in config/jobs/kubernetes/wg-k8s-infra/trusted/
	jobs := []cfg.JobBase{}
	for _, job := range c.AllStaticPostsubmits(nil) {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range c.AllPeriodics() {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range jobs {
		if job.Cluster != trusted {
			continue
		}
		if strings.HasPrefix(job.SourcePath, imagePushingDir) {
			if err := validateImagePushingImage(job.Spec); err != nil {
				t.Errorf("%s defined in %s %s", job.Name, job.SourcePath, err)
			}
		} else if !strings.HasPrefix(job.SourcePath, trustedPath) {
			t.Errorf("%s defined in %s may not run in cluster: %s", job.Name, job.SourcePath, trusted)
		}
	}
}

func validateImagePushingImage(spec *coreapi.PodSpec) error {
	const imagePushingImage = "gcr.io/k8s-testimages/image-builder"

	for _, c := range spec.Containers {
		if !strings.HasPrefix(c.Image, imagePushingImage+":") {
			return fmt.Errorf("must use a pinned version of %s", imagePushingImage)
		}
	}

	return nil
}

// Restrict the use of specific secrets to certain jobs in config/jobs/<org>/<project>/<basename>.yaml
func TestTrustedJobSecretsRestricted(t *testing.T) {
	type labels map[string]string

	getSecretsFromPreset := func(labels labels) sets.String {
		secrets := sets.NewString()
		for _, preset := range c.Presets {
			match := true
			for k, v1 := range preset.Labels {
				// check if a given list of labels matches all labels from this preset
				if v2, ok := labels[k]; !ok || v1 != v2 {
					match = false
					break
				}
			}
			if match {
				for _, v := range preset.Volumes {
					if v.VolumeSource.Secret != nil {
						secrets.Insert(v.VolumeSource.Secret.SecretName)
					}
				}
				for _, e := range preset.Env {
					if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
						secrets.Insert(e.ValueFrom.SecretKeyRef.Name)
					}
				}
			}
		}
		return secrets
	}

	secretsRestricted := map[string]struct {
		secrets            sets.String
		isTrusted          bool
		allowedInPresubmit bool
	}{
		"kubernetes-sigs/sig-storage-local-static-provisioner": {secrets: sets.NewString("sig-storage-local-static-provisioner-pusher"), isTrusted: true},
		"kubernetes-csi/csi-driver-nfs":                        {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-csi/csi-driver-smb":                        {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/azuredisk-csi-driver":                 {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/azurefile-csi-driver":                 {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/blob-csi-driver":                      {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/cloud-provider-azure":                 {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/image-builder":                        {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/secrets-store-csi-driver":             {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes-sigs/sig-windows":                          {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes/sig-cloud-provider":                        {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes/sig-network":                               {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"}), allowedInPresubmit: true},
		"kubernetes/sig-release":                               {secrets: getSecretsFromPreset(labels{"preset-azure-cred": "true"})},
		"kubernetes-sigs/cluster-api-provider-azure":           {secrets: getSecretsFromPreset(labels{"preset-azure-cred-only": "true"}), allowedInPresubmit: true},
	}
	allSecrets := sets.String{}
	for _, s := range secretsRestricted {
		allSecrets.Insert(s.secrets.List()...)
	}

	isSecretUsedByContainer := func(secret string, container coreapi.Container) bool {
		if container.EnvFrom == nil {
			return false
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil && envFrom.SecretRef.Name == secret {
				return true
			}
		}
		return false
	}

	isSecretUsed := func(secret string, job cfg.JobBase) bool {
		if job.Spec == nil {
			return false
		}
		if job.Spec.Volumes != nil {
			for _, v := range job.Spec.Volumes {
				if v.VolumeSource.Secret != nil && v.VolumeSource.Secret.SecretName == secret {
					return true
				}
			}
		}
		if job.Spec.Containers != nil {
			for _, c := range job.Spec.Containers {
				if isSecretUsedByContainer(secret, c) {
					return true
				}
			}
		}
		if job.Spec.InitContainers != nil {
			for _, c := range job.Spec.InitContainers {
				if isSecretUsedByContainer(secret, c) {
					return true
				}
			}
		}
		// iterate all presets because they can also reference secrets
		secretsFromPreset := getSecretsFromPreset(labels(job.Labels))
		return secretsFromPreset.Has(secret)
	}

	getJobOrgProjectBasename := func(path string) (string, string, string) {
		cleanPath := strings.Trim(strings.TrimPrefix(path, *jobConfigPath), string(filepath.Separator))
		seps := strings.Split(cleanPath, string(filepath.Separator))
		if len(seps) <= 2 {
			return "", "", ""
		}
		return seps[0], seps[1], seps[2]
	}

	// Most presubmit jobs should not use any restricted secrets.
	for _, job := range c.AllStaticPresubmits(nil) {
		if job.Cluster != prowapi.DefaultClusterAlias {
			// check against default public cluster only
			continue
		}
		// check if this presubmit job is allowed to use the secret
		org, project, _ := getJobOrgProjectBasename(job.SourcePath)
		s, ok := secretsRestricted[filepath.Join(org, project)]
		allowedInPresubmit := ok && s.allowedInPresubmit
		for _, secret := range allSecrets.List() {
			if isSecretUsed(secret, job.JobBase) && !allowedInPresubmit {
				t.Errorf("%q defined in %q may not use secret %q in %q cluster", job.Name, job.SourcePath, secret, job.Cluster)
			}
		}
	}

	secretsCanUseByPath := func(path string) sets.String {
		org, project, basename := getJobOrgProjectBasename(path)
		s, ok := secretsRestricted[filepath.Join(org, project)]
		if !ok || (s.isTrusted && basename != fmt.Sprintf("%s-trusted.yaml", project)) {
			return nil
		}
		return s.secrets
	}

	// Postsubmit/periodic jobs defined in
	// config/jobs/<org>/<project>/<project>-trusted.yaml can and only can use restricted
	// secrets for <org>/repo>.
	jobs := []cfg.JobBase{}
	for _, job := range c.AllStaticPostsubmits(nil) {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range c.AllPeriodics() {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range jobs {
		if job.Cluster != prowapi.DefaultClusterAlias {
			// check against default public cluster only
			continue
		}
		secretsCanUse := secretsCanUseByPath(job.SourcePath)
		for _, secret := range allSecrets.List() {
			if secretsCanUse != nil && secretsCanUse.Has(secret) {
				t.Logf("allow secret %v for job %s defined in %s", secret, job.Name, job.SourcePath)
				continue
			}
			if isSecretUsed(secret, job) {
				t.Errorf("%q defined in %q may not use secret %q in %q cluster", job.Name, job.SourcePath, secret, job.Cluster)
			}
		}
	}
}

// Unit test jobs outside kubernetes-security do not use the security cluster
// and that jobs inside kubernetes-security DO
func TestConfigSecurityClusterRestricted(t *testing.T) {
	for repo, jobs := range c.PresubmitsStatic {
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
	for repo, jobs := range c.PostsubmitsStatic {
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
	// TODO: this will need to be more complex if we ever add k-s periodic
	for _, job := range c.AllPeriodics() {
		if job.Cluster == "security" {
			t.Fatalf("Jobs not in kubernetes-security/* should not use the security cluster! %s", job.Name)
		}
	}
}

// checkDockerSocketVolumes returns an error if any volume uses a hostpath
// to the docker socket. we do not want to allow this
func checkDockerSocketVolumes(volumes []coreapi.Volume) error {
	for _, volume := range volumes {
		if volume.HostPath != nil && volume.HostPath.Path == "/var/run/docker.sock" {
			return errors.New("job uses HostPath with docker socket")
		}
	}
	return nil
}

// Make sure jobs are not using the docker socket as a host path
func TestJobDoesNotHaveDockerSocket(t *testing.T) {
	for _, presubmit := range c.AllStaticPresubmits(nil) {
		if presubmit.Spec != nil {
			if err := checkDockerSocketVolumes(presubmit.Spec.Volumes); err != nil {
				t.Errorf("Error in presubmit: %v", err)
			}
		}
	}

	for _, postsubmit := range c.AllStaticPostsubmits(nil) {
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
func checkLatestUsesImagePullPolicy(spec *coreapi.PodSpec) error {
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
	for _, presubmit := range c.AllStaticPresubmits(nil) {
		if presubmit.Spec != nil {
			if err := checkLatestUsesImagePullPolicy(presubmit.Spec); err != nil {
				t.Errorf("Error in presubmit %q: %v", presubmit.Name, err)
			}
		}
	}

	for _, postsubmit := range c.AllStaticPostsubmits(nil) {
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
func checkKubekinsPresets(jobName string, spec *coreapi.PodSpec, labels map[string]string, validLabels map[string]bool) error {
	service := true
	ssh := true

	for _, container := range spec.Containers {
		if strings.Contains(container.Image, "kubekins-e2e") || strings.Contains(container.Image, "bootstrap") {
			service = false
			for key, val := range labels {
				if key == "preset-service-account" && val == "true" {
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
		pair := key + ":" + val
		if validVal, ok := validLabels[pair]; !ok || !validVal {
			return fmt.Errorf("key-value pair %s is not found in list of valid presets list", pair)
		}
	}

	return nil
}

// TestValidPresets makes sure all presets name starts with 'preset-', all job presets are valid,
// and jobs that uses kubekins-e2e image has the right service account preset
func TestValidPresets(t *testing.T) {
	validLabels := map[string]bool{}
	for _, preset := range c.Presets {
		for label, val := range preset.Labels {
			if !strings.HasPrefix(label, "preset-") {
				t.Errorf("Preset label %s - label name should start with 'preset-'", label)
			}
			pair := label + ":" + val
			if _, ok := validLabels[pair]; ok {
				t.Errorf("Duplicated preset 'label:value' pair : %s", pair)
			} else {
				validLabels[pair] = true
			}
		}
	}

	if !*k8sProw {
		return
	}

	for _, presubmit := range c.AllStaticPresubmits(nil) {
		if presubmit.Spec != nil && !*presubmit.Decorate {
			if err := checkKubekinsPresets(presubmit.Name, presubmit.Spec, presubmit.Labels, validLabels); err != nil {
				t.Errorf("Error in presubmit %q: %v", presubmit.Name, err)
			}
		}
	}

	for _, postsubmit := range c.AllStaticPostsubmits(nil) {
		if postsubmit.Spec != nil && !*postsubmit.Decorate {
			if err := checkKubekinsPresets(postsubmit.Name, postsubmit.Spec, postsubmit.Labels, validLabels); err != nil {
				t.Errorf("Error in postsubmit %q: %v", postsubmit.Name, err)
			}
		}
	}

	for _, periodic := range c.AllPeriodics() {
		if periodic.Spec != nil && !*periodic.Decorate {
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
	useSharedBuildInArgs := hasArg("--use-shared-build", args)
	extractInArgs := hasArg("--extract", args)
	buildInArgs := hasArg("--build", args)

	if useSharedBuildInArgs && extractInArgs {
		return fmt.Errorf("job %s: --use-shared-build and --extract cannot be combined", jobName)
	}

	if useSharedBuildInArgs && buildInArgs {
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

	// test_args should not have double slashes on ginkgo flags
	for _, arg := range args {
		ginkgoArgs := ""
		if strings.HasPrefix(arg, "--test_args=") {
			split := strings.SplitN(arg, "=", 2)
			ginkgoArgs = split[1]
		} else if strings.HasPrefix(arg, "--upgrade_args=") {
			split := strings.SplitN(arg, "=", 2)
			ginkgoArgs = split[1]
		}

		if strings.Contains(ginkgoArgs, "\\\\") {
			return fmt.Errorf("jobs %s - double slashes in ginkgo args should be single slash now : arg %s", jobName, arg)
		}
	}

	// timeout should be valid
	bootstrapTimeout := 0 * time.Minute
	kubetestTimeout := 0 * time.Minute
	var err error
	kubetest := false
	for _, arg := range args {
		if strings.HasPrefix(arg, "--timeout=") {
			timeout := strings.SplitN(arg, "=", 2)[1]
			if kubetest {
				if kubetestTimeout, err = time.ParseDuration(timeout); err != nil {
					return fmt.Errorf("jobs %s - invalid kubetest timeout : arg %s", jobName, arg)
				}
			} else {
				if bootstrapTimeout, err = time.ParseDuration(timeout + "m"); err != nil {
					return fmt.Errorf("jobs %s - invalid bootstrap timeout : arg %s", jobName, arg)
				}
			}
		}

		if arg == "--" {
			kubetest = true
		}
	}

	if bootstrapTimeout.Minutes()-kubetestTimeout.Minutes() < 20.0 {
		return fmt.Errorf(
			"jobs %s - kubetest timeout(%v), bootstrap timeout(%v): bootstrap timeout need to be 20min more than kubetest timeout!", jobName, kubetestTimeout, bootstrapTimeout)
	}

	return nil
}

// TestValidScenarioArgs makes sure all scenario args in job configs are valid
func TestValidScenarioArgs(t *testing.T) {
	for _, job := range c.AllStaticPresubmits(nil) {
		if job.Spec != nil && !*job.Decorate {
			if err := checkScenarioArgs(job.Name, job.Spec.Containers[0].Image, job.Spec.Containers[0].Args); err != nil {
				t.Errorf("Invalid Scenario Args : %s", err)
			}
		}
	}

	for _, job := range c.AllStaticPostsubmits(nil) {
		if job.Spec != nil && !*job.Decorate {
			if err := checkScenarioArgs(job.Name, job.Spec.Containers[0].Image, job.Spec.Containers[0].Args); err != nil {
				t.Errorf("Invalid Scenario Args : %s", err)
			}
		}
	}

	for _, job := range c.AllPeriodics() {
		if job.Spec != nil && !*job.Decorate {
			if err := checkScenarioArgs(job.Name, job.Spec.Containers[0].Image, job.Spec.Containers[0].Args); err != nil {
				t.Errorf("Invalid Scenario Args : %s", err)
			}
		}
	}
}

func allStaticJobs() []cfg.JobBase {
	jobs := []cfg.JobBase{}
	for _, job := range c.AllStaticPresubmits(nil) {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range c.AllStaticPostsubmits(nil) {
		jobs = append(jobs, job.JobBase)
	}
	for _, job := range c.AllPeriodics() {
		jobs = append(jobs, job.JobBase)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Name < jobs[j].Name
	})
	return jobs
}

func verifyPodQOSGuaranteed(spec *coreapi.PodSpec, required bool) (errs []error) {
	should := "should"
	if required {
		should = "must"
	}
	resourceNames := []coreapi.ResourceName{
		coreapi.ResourceCPU,
		coreapi.ResourceMemory,
	}
	zero := resource.MustParse("0")
	for _, c := range spec.Containers {
		for _, r := range resourceNames {
			limit, ok := c.Resources.Limits[r]
			if !ok {
				errs = append(errs, fmt.Errorf("container '%v' %v have resources.limits[%v] specified", c.Name, should, r))
			}
			request, ok := c.Resources.Requests[r]
			if !ok {
				errs = append(errs, fmt.Errorf("container '%v' %v have resources.requests[%v] specified", c.Name, should, r))
			}
			if limit.Cmp(zero) == 0 {
				errs = append(errs, fmt.Errorf("container '%v' resources.limits[%v] %v be non-zero", c.Name, r, should))
			} else if limit.Cmp(request) != 0 {
				errs = append(errs, fmt.Errorf("container '%v' resources.limits[%v] (%v) %v match request (%v)", c.Name, r, limit.String(), should, request.String()))
			}
		}
	}
	return errs
}

// A job is merge-blocking if it:
// - is not optional
// - reports (aka does not skip reporting)
// - always runs OR runs if some path changed
func isMergeBlocking(job cfg.Presubmit) bool {
	return !job.Optional && !job.SkipReport && (job.AlwaysRun || job.RunIfChanged != "")
}

func isKubernetesReleaseBlocking(job cfg.JobBase) bool {
	re := regexp.MustCompile(`sig-release-(1.[0-9]{2}|master)-blocking`)
	dashboards, ok := job.Annotations["testgrid-dashboards"]
	if !ok {
		return false
	}
	return re.MatchString(dashboards)
}

func TestKubernetesMergeBlockingJobsCIPolicy(t *testing.T) {
	repo := "kubernetes/kubernetes"
	jobs := c.AllStaticPresubmits([]string{repo})
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Name < jobs[j].Name
	})
	for _, job := range jobs {
		// Only consider Pods that are merge-blocking
		if job.Spec == nil || !isMergeBlocking(job) {
			continue
		}
		// job Pod must qualify for Guaranteed QoS
		errs := verifyPodQOSGuaranteed(job.Spec, true)
		// jobs must run on k8s-infra-prow-build cluster
		if job.Cluster != "k8s-infra-prow-build" {
			errs = append(errs, fmt.Errorf("must run in cluster: k8s-infra-prow-build, found: %v", job.Cluster))
		}
		branches := job.Branches
		for _, err := range errs {
			t.Errorf("%v (%v): %v", job.Name, branches, err)
		}
	}
}

func TestKubernetesReleaseBlockingJobsCIPolicy(t *testing.T) {
	for _, job := range allStaticJobs() {
		// Only consider Pods that are release-blocking
		if job.Spec == nil || !isKubernetesReleaseBlocking(job) {
			continue
		}
		// job Pod must qualify for Guaranteed QoS
		errs := verifyPodQOSGuaranteed(job.Spec, true)
		// jobs must run on k8s-infra-prow-build cluster
		if job.Cluster != "k8s-infra-prow-build" {
			errs = append(errs, fmt.Errorf("must run in cluster: k8s-infra-prow-build, found: %v", job.Cluster))
		}
		for _, err := range errs {
			t.Errorf("%v: %v", job.Name, err)
		}
	}
}

func TestK8sInfraProwBuildJobsCIPolicy(t *testing.T) {
	jobs := allStaticJobs()
	for _, job := range jobs {
		// Only consider Pods destined for the k8s-infra-prow-builds cluster
		if job.Spec == nil || job.Cluster != "k8s-infra-prow-build" {
			continue
		}
		// job Pod must qualify for Guaranteed QoS
		errs := verifyPodQOSGuaranteed(job.Spec, true)
		for _, err := range errs {
			t.Errorf("%v: %v", job.Name, err)
		}
	}
}

// Fast builds take 20-30m, cross builds take 90m-2h. We want to pick up builds
// containing the latest merged PRs as soon as possible for the in-development release
func TestSigReleaseMasterBlockingOrInformingJobsMustUseFastBuilds(t *testing.T) {
	jobs := allStaticJobs()
	for _, job := range jobs {
		dashboards, ok := job.Annotations["testgrid-dashboards"]
		if !ok || !strings.Contains(dashboards, "sig-release-master-blocking") || !strings.Contains(dashboards, "sig-release-master-informing") {
			continue
		}
		extract := ""
		for _, arg := range job.Spec.Containers[0].Args {
			if strings.HasPrefix(arg, "--extract=") {
				extract = strings.TrimPrefix(arg, "--extract=")
				if extract != "ci/latest-fast" {
					t.Errorf("%s: release-master-blocking e2e jobs must use --extract=ci/latest-fast, found --extract=ci/latest instead", job.Name)
				}
			}
		}
	}
}

// matches regex used by the "version" extractMode defined in kubetest/extract_k8s.go
var kubetestVersionExtractModeRegex = regexp.MustCompile(`^(v\d+\.\d+\.\d+[\w.\-+]*)$`)

// extractUsesCIBucket returns true if kubetest --extract=foo
// would use the value of --extract-ci-bucket, false otherwise
func extractUsesCIBucket(extract string) bool {
	if strings.HasPrefix(extract, "ci/") || strings.HasPrefix(extract, "gci/") {
		return true
	}
	mat := kubetestVersionExtractModeRegex.FindStringSubmatch(extract)
	if mat != nil {
		version := mat[1]
		// non-gke versions that include a + are CI builds
		return !strings.Contains(version, "-gke.") && strings.Contains(version, "+")
	}
	return false
}

// extractUsesReleaseBucket returns true if kubetest --extract=foo
// would use the value of --extract-release-bucket, false otherwise
func extractUsesReleaseBucket(extract string) bool {
	if strings.HasPrefix(extract, "release/") {
		return true
	}
	mat := kubetestVersionExtractModeRegex.FindStringSubmatch(extract)
	if mat != nil {
		version := mat[1]
		// non-gke versions that lack a + are release builds
		return !strings.Contains(version, "-gke.") && !strings.Contains(version, "+")
	}
	return false
}

// To help with migration to community-owned buckets for CI and release artifacts:
// - jobs using --extract=ci/latest-fast MUST pull from gs://k8s-release-dev
// - release-blocking jobs using --extract=ci/*  MUST from pull gs://k8s-release-dev
// TODO(https://github.com/kubernetes/k8s.io/issues/846): switch from SHOULD to MUST once all jobs migrated
// - jobs using --extract=ci/* SHOULD pull from gs://k8s-release-dev
// TODO(https://github.com/kubernetes/k8s.io/issues/1569): start warning once gs://k8s-release populated
// - jobs using --extract=release/* SHOULD pull from gs://k8s-release
func TestKubernetesE2eJobsMustExtractFromK8sInfraBuckets(t *testing.T) {
	jobs := allStaticJobs()
	var totalJobs, needsFixJobs int
	for _, job := range jobs {
		needsFix := false
		extracts := []string{}
		const (
			defaultCIBucket       = "kubernetes-release-dev" // ensure this matches kubetest --extract-ci-bucket default
			expectedCIBucket      = "k8s-release-dev"
			defaultReleaseBucket  = "kubernetes-release" // ensure this matches kubetest --extract-release-bucket default
			expectedReleaseBucket = "k8s-release"
			k8sReleaseIsPopulated = false // TODO(kubernetes/k8s.io#1569): drop this once gs://k8s-release populated
		)
		ciBucket := defaultCIBucket
		releaseBucket := defaultReleaseBucket
		for _, container := range job.Spec.Containers {
			for _, arg := range container.Args {
				if strings.HasPrefix(arg, "--extract=") {
					extracts = append(extracts, strings.TrimPrefix(arg, "--extract="))
				}
				if strings.HasPrefix(arg, "--extract-ci-bucket=") {
					ciBucket = strings.TrimPrefix(arg, "--extract-ci-bucket=")
				}
				if strings.HasPrefix(arg, "--extract-release-bucket=") {
					releaseBucket = strings.TrimPrefix(arg, "--extract-release-bucket=")
				}
			}
			for _, extract := range extracts {
				fail := false
				if extractUsesCIBucket(extract) && ciBucket != expectedCIBucket {
					needsFix = true
					jobDesc := "jobs"
					fail = extract == "ci/latest-fast"
					if isKubernetesReleaseBlocking(job) {
						fail = true
						jobDesc = "release-blocking jobs"
					}
					msg := fmt.Sprintf("%s: %s using --extract=%s must have --extract-ci-bucket=%s", job.Name, jobDesc, extract, expectedCIBucket)
					if fail {
						t.Errorf("FAIL - %s", msg)
					} else {
						t.Logf("WARN - %s", msg)
					}
				}
				if k8sReleaseIsPopulated && extractUsesReleaseBucket(extract) && releaseBucket != expectedReleaseBucket {
					needsFix = true
					jobDesc := "jobs"
					if isKubernetesReleaseBlocking(job) {
						fail = true
						jobDesc = "release-blocking jobs"
					}
					fail := isKubernetesReleaseBlocking(job)
					msg := fmt.Sprintf("%s: %s using --extract=%s must have --extract-release-bucket=%s", job.Name, jobDesc, extract, expectedCIBucket)
					if fail {
						t.Errorf("FAIL - %s", msg)
					} else {
						t.Logf("WARN - %s", msg)
					}
				}
			}
		}
		totalJobs++
		if needsFix {
			needsFixJobs++
		}
	}
	if needsFixJobs > 0 {
		t.Logf("%4d/%4d jobs should be updated to pull from community-owned gcs buckets", needsFixJobs, totalJobs)
	}
}

// Prow jobs should use pod-utils instead of relying on bootstrap
// https://github.com/kubernetes/test-infra/issues/20760
func TestKubernetesProwJobsShouldUsePodUtils(t *testing.T) {
	total := 0
	fails := 0
	for _, job := range allStaticJobs() {
		total++
		// Only consider Pods
		// TODO(https://github.com/kubernetes/test-infra/issues/14343): remove kubeflow exemption when job configs migrated
		if job.Spec == nil || strings.HasPrefix("kubeflow", job.Name) {
			continue
		}
		if !*job.Decorate {
			// bootstrap jobs don't use multiple containers
			container := job.Spec.Containers[0]
			repos := []string{}
			scenario := ""
			for _, arg := range container.Args {
				if strings.HasPrefix(arg, "--repo=") {
					repos = append(repos, strings.TrimPrefix(arg, "--repo="))
				}
				if strings.HasPrefix(arg, "--scenario=") {
					scenario = strings.TrimPrefix(arg, "--scenario=")
				}
			}
			fails++
			if len(repos) > 0 {
				t.Logf("%v: %v: should use pod-utils, found bootstrap args to clone: %v", job.SourcePath, job.Name, repos)
			} else if scenario != "" {
				t.Logf("%v: %v: should use pod-utils, found --scenario=%v, implies clone: [kubernetes/test-infra]", job.SourcePath, job.Name, scenario)
			} else {
				t.Logf("%v: %v: should use pod-utils, unknown case", job.SourcePath, job.Name)
			}
		}
	}
	if fails > 0 {
		t.Logf("%v/%v jobs do not use pod-utils", fails, total)
	}
}

// Prow jobs should use kubetest2 instead of deprecated scenarios
// https://github.com/kubernetes/test-infra/tree/master/scenarios#deprecation-notice
func TestKubernetesProwJobsShouldNotUseDeprecatedScenarios(t *testing.T) {
	total := 0
	fails := 0
	for _, job := range allStaticJobs() {
		total++
		// Only consider Pods
		if job.Spec == nil {
			continue
		}
		// bootstrap jobs don't use multiple containers
		container := job.Spec.Containers[0]
		// might also be good proxy for "relies on bootstrap"
		// if strings.Contains(container.Image, "kubekins-e2e") || strings.Contains(container.Image, "bootstrap")
		scenario := ""
		r, _ := regexp.Compile(".*/scenarios/([a-z0-9_]+).py.*")
		for _, cmd := range container.Command {
			if submatches := r.FindStringSubmatch(cmd); submatches != nil {
				scenario = submatches[1]
			}
		}
		if scenario != "" {
			fails++
			t.Logf("%v: %v: should not be using deprecated scenarios, is directly invoking: %v", job.SourcePath, job.Name, scenario)
			continue
		}
		for _, arg := range container.Args {
			if strings.HasPrefix(arg, "--scenario=") {
				scenario = strings.TrimPrefix(arg, "--scenario=")
			}
		}
		if scenario != "" {
			fails++
			t.Logf("%v: %v: should not be using deprecated scenarios, is invoking via bootrap: %v", job.SourcePath, job.Name, scenario)
		}
	}
	if fails > 0 {
		t.Logf("%v/%v jobs using deprecated scenarios", fails, total)
	}
}
